import json
import math
import os
import time
import uuid
from client import Client, wait_ready


def main():
    c = Client()
    wait_ready(c)
    actor = c.login()
    run = uuid.uuid4().hex[:12]
    hold = int(os.environ.get('HOLD_SECONDS', 60))
    ramp = int(os.environ.get('RAMP_SECONDS', 15))
    rate = int(os.environ.get('JOB_RATE', 12))
    # Upper bound at 4x for the entire run, plus spare fixtures. Never reuse IDs.
    count = math.ceil(4 * rate * (4 * hold + 3 * ramp) / 60) + 10
    rows = int(os.environ.get('JOB_ROWS', 1000))
    delay = float(os.environ.get('JOB_DELAY', 2))
    if min(hold, ramp, rate, rows) <= 0 or not 0 <= delay <= 60:
        raise ValueError('Positive rates/durations/rows required; JOB_DELAY must be 0..60')
    for setting, default in [('API_RATE', 10), ('MAX_VUS', 20), ('DRAIN_SECONDS', 300)]:
        if int(os.environ.get(setting, default)) <= 0:
            raise ValueError(f'{setting} must be a positive integer')
    if float(os.environ.get('API_P95_MS', 500)) <= 0 or not 0 < float(os.environ.get('ERROR_RATE', 0.01)) <= 1:
        raise ValueError('API_P95_MS must be positive and ERROR_RATE in (0,1]')
    mode = os.environ.get('MODE', 'same')
    if mode not in ('same', 'different'):
        raise ValueError('MODE must be same or different')
    group = c.call('POST', '/api/v1/admin/groups', {'name': f'k6-{run}', 'leader_id': actor['id']})
    c.call('PUT', f"/api/v1/admin/groups/{group['id']}/members/{actor['id']}", {})
    c.login()  # Membership changes intentionally revoke affected sessions.
    databases = []
    table = f'k6_{run}'

    def approve(db, name, sql, down):
        path = f'/api/v1/databases/{db}/migrations'
        m = c.call('POST', path, {'name': name, 'sql_text': sql, 'sql_down': down})
        path += '/' + m['id']
        c.call('POST', path + '/submit-review', {})
        approved = c.call('POST', path + '/approve', {'comment': 'Isolated k6 fixture'})
        if approved['status'] != 'APPROVED':
            raise RuntimeError('Fixture did not receive approval')
        return m['id']

    for host in (['target-a'] if mode == 'same' else ['target-a', 'target-b']):
        db = c.call('POST', '/api/v1/admin/databases', {
            'engine': 'postgres', 'label': f'k6-{run}-{host}', 'host': host, 'port': 5432,
            'database_name': 'stress', 'ssl_mode': 'disable',
            'reader_username': 'stress', 'reader_password': 'stress',
            'migrator_username': 'stress', 'migrator_password': 'stress'})['id']
        databases.append(db)
        c.call('PUT', f"/api/v1/admin/groups/{group['id']}/databases/{db}/access", {'access_level': 'VIEW_EDIT'})
        c.login()  # Grant changes also revoke affected sessions.
        bootstrap = approve(db, 'k6 bootstrap', f'CREATE TABLE {table} (job_no integer, n integer, payload text);', f'DROP TABLE {table};')
        job = c.call('POST', f'/api/v1/databases/{db}/migrations/{bootstrap}/execute', {})['job_id']
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            state = c.call('GET', f'/api/v1/databases/{db}/jobs/{job}')['status']
            if state == 'SUCCEEDED':
                break
            if state in ('FAILED', 'BLOCKED'):
                raise RuntimeError(f'Bootstrap job {state}; inspect worker logs')
            time.sleep(1)
        else:
            raise RuntimeError('Bootstrap job timed out; verify worker is running')
    fixtures = []
    for i in range(count):
        db = databases[i % len(databases)]
        # Independent insert sets; delay is explicit synthetic execution time, not a dataset benchmark.
        sql = f"INSERT INTO {table} SELECT {i}, n, repeat('x', 128) FROM generate_series(1,{rows}) n; SELECT pg_sleep({delay});"
        mid = approve(db, f'k6 job {i}', sql, f'DELETE FROM {table} WHERE job_no = {i};')
        fixtures.append({'database': db, 'migration': mid})
    with open('/results/fixture.json', 'w') as f:
        json.dump({'run': run, 'mode': mode, 'databases': databases, 'table': table,
                   'jobs': fixtures, 'bootstrap_ids': [j['id'] for db in databases for j in c.jobs(db)],
                   'hold': hold, 'ramp': ramp, 'job_rate': rate, 'rows': rows, 'delay': delay}, f)
    print(f'Prepared {count} APPROVED migrations, mode={mode}', flush=True)


if __name__ == '__main__':
    main()
