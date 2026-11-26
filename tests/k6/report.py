import datetime
import json
import math
import os
from pathlib import Path
import time
from client import Client


def percentile(values, p):
    if not values:
        return None
    v = sorted(values)
    i = (len(v) - 1) * p / 100
    return v[math.floor(i)] + (v[math.ceil(i)] - v[math.floor(i)]) * (i % 1)


def timestamp(value):
    return datetime.datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp()


def phase_at(seconds, windows):
    return next((name for name, start, end in windows if start <= seconds < end), 'drain')


def backlog_at(jobs, instant):
    return sum(timestamp(j['created_at']) < instant and
               (not j.get('finished_at') or timestamp(j['finished_at']) > instant) for j in jobs)


def job_stats(jobs):
    return {
        'succeeded': sum(j['status'] == 'SUCCEEDED' for j in jobs),
        'failed': sum(j['status'] in ('FAILED', 'BLOCKED') for j in jobs),
        'unfinished': sum(j['status'] not in ('SUCCEEDED', 'FAILED', 'BLOCKED') for j in jobs),
        'queue_ms': [(timestamp(j['started_at']) - timestamp(j['created_at'])) * 1000
                     for j in jobs if j.get('started_at')],
        'duration_ms': [(timestamp(j['finished_at']) - timestamp(j['started_at'])) * 1000
                        for j in jobs if j.get('started_at') and j.get('finished_at')],
    }


def main():
    root = Path('/results')
    f = json.loads((root / 'fixture.json').read_text())
    c = Client()
    c.login()
    deadline = time.monotonic() + int(os.environ.get('DRAIN_SECONDS', 300))
    while True:
        jobs = [j for db in f['databases'] for j in c.jobs(db) if j['id'] not in f['bootstrap_ids']]
        pending = job_stats(jobs)['unfinished']
        if not pending or time.monotonic() >= deadline:
            break
        print(f'Draining {pending} unfinished jobs...', flush=True)
        time.sleep(2)
    (root / 'jobs.json').write_text(json.dumps(jobs, indent=2))
    windows, end = [], 0
    for name, duration in [('normal', f['hold']), ('ramp_2x', f['ramp']), ('2x', f['hold']),
                           ('ramp_4x', f['ramp']), ('4x', f['hold']), ('ramp_down', f['ramp']),
                           ('recovery', f['hold'])]:
        windows.append((name, end, end + duration))
        end += duration
    metrics = {n: {'latency': [], 'errors': [], 'polls': 0} for n, _, _ in windows}
    metrics['drain'] = {'latency': [], 'errors': [], 'polls': 0}
    start, drops = None, 0
    with (root / 'raw.json').open() as stream:
        for line in stream:
            record = json.loads(line)
            if record.get('type') != 'Point':
                continue
            metric, data = record['metric'], record['data']
            tags = data.get('tags') or {}
            if metric == 'run_origin_ms':
                start = data['value'] / 1000
            if metric == 'dropped_iterations':
                drops += data['value']
            phase = tags.get('phase')
            if phase not in metrics:
                continue
            if tags.get('kind') == 'api':
                if metric == 'http_req_duration':
                    metrics[phase]['latency'].append(data['value'])
                elif metric == 'http_req_failed':
                    metrics[phase]['errors'].append(data['value'])
            elif tags.get('kind') == 'poll' and metric == 'http_req_duration':
                metrics[phase]['polls'] += 1
    if start is None:
        raise RuntimeError('k6 did not emit run_origin_ms; no valid run to report')
    elapsed = max(end, time.time() - start)
    windows.append(('drain', end, elapsed + 0.001))
    rows = []
    p95_limit = float(os.environ.get('API_P95_MS', 500))
    error_limit = float(os.environ.get('ERROR_RATE', 0.01))
    def fmt(v):
        return '—' if v is None else f'{v:.2f}'
    lines = [f"### k6: {f['mode']} — run {f['run']}", '',
             f"Workload: API normal {os.environ.get('API_RATE', '10')} iterations/s; migration normal {f['job_rate']}/min; "
             f"{f['rows']} inserted rows/job + synthetic pg_sleep({f['delay']}) seconds; worker concurrency {os.environ.get('JOBS_CONCURRENCY', '4')}.", '',
             f'Threshold API: p95 < {p95_limit} ms; error rate < {error_limit * 100}%. Dropped iterations: {drops:g}.', '',
             '| Fase | API p50 ms | API p95 ms | req/s | Error % | Queue p95 ms* | Job duration p95 ms* | Sukses selesai | job/s | Gagal* | Belum selesai* | Backlog akhir fase |',
             '|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|']
    for name, begin, finish in windows:
        m = metrics.get(name, {'latency': [], 'errors': [], 'polls': 0})
        submitted = [j for j in jobs if phase_at(timestamp(j['created_at']) - start, windows) == name]
        stats = job_stats(submitted)
        finished = sum(j['status'] == 'SUCCEEDED' and j.get('finished_at') and
                       phase_at(timestamp(j['finished_at']) - start, windows) == name for j in jobs)
        errors = sum(m['errors']) / len(m['errors']) if m['errors'] else None
        p95 = percentile(m['latency'], 95)
        row = dict(phase=name, seconds=finish-begin, api_p50=percentile(m['latency'], 50), api_p95=p95,
                   rps=len(m['latency'])/(finish-begin), error_rate=errors, poll_requests=m['polls'],
                   completed=finished, jobs_per_second=finished/(finish-begin),
                   backlog_end=backlog_at(jobs, start + finish), **stats)
        rows.append(row)
        lines.append(f"| {name} | {fmt(row['api_p50'])} | {fmt(p95)} | {row['rps']:.2f} | {fmt(None if errors is None else errors*100)} | {fmt(percentile(stats['queue_ms'],95))} | {fmt(percentile(stats['duration_ms'],95))} | {finished} | {row['jobs_per_second']:.3f} | {stats['failed']} | {stats['unfinished']} | {row['backlog_end']} |")
    lines += ['', '*Queue/durasi/status akhir dikelompokkan berdasarkan fase pengiriman; throughput berdasarkan waktu selesai server. '
              'Durasi mencakup retry sejak started_at pertama. Polling/auth terpisah dari metrik API. Bootstrap dikecualikan.', '', 'Analisis otomatis:']
    for row in rows:
        if row['phase'] == 'drain':
            continue
        if row['api_p95'] is None or row['error_rate'] is None:
            lines.append(f"- {row['phase']}: sampel API tidak tersedia; hasil tidak dapat dinilai.")
        else:
            passed = row['api_p95'] < p95_limit and row['error_rate'] < error_limit
            lines.append(f"- {row['phase']}: {'memenuhi' if passed else 'melewati'} batas API; "
                         f"queue p95 {fmt(percentile(row['queue_ms'],95))} ms, "
                         f"{row['completed']} job selesai, backlog akhir fase {row['backlog_end']}, {row['unfinished']} job belum selesai setelah drain.")
    normal, recovery = rows[0], rows[6]
    if normal['api_p95'] and recovery['api_p95']:
        lines.append(f"- Recovery/normal p95 = {recovery['api_p95']/normal['api_p95']:.2f}×. "
                     'Nilai mendekati 1 disertai backlog yang turun mendukung pemulihan; periksa juga queue, error, dan log worker.')
    stats = job_stats(jobs)
    lines += [f"- Rekonsiliasi: {len(jobs)} job diterima, {stats['succeeded']} SUCCEEDED, {stats['failed']} FAILED/BLOCKED, {stats['unfinished']} unfinished.",
              '- HTTP submit sukses tidak dihitung sebagai migration sukses. HTTP timeout dapat tetap menghasilkan job; jobs.json adalah rekonsiliasi server.',
              '- Beban sintetis ini bukan bukti benchmark UPDATE >1 detik atau pengujian batching; keduanya perlu pengujian dataset terpisah.', '']
    (root / 'report.md').write_text('\n'.join(lines))
    (root / 'report.json').write_text(json.dumps({'fixture': f['run'], 'mode': f['mode'], 'dropped': drops, 'phases': rows}, indent=2))
    print('\n'.join(lines), flush=True)
    if not jobs or stats['failed'] or stats['unfinished'] or drops:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
