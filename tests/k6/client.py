import http.cookiejar
import json
import os
import time
import urllib.error
import urllib.request


class Client:
    def __init__(self):
        self.base = os.environ.get('BASE_URL', 'http://backend:8080')
        self.cookies = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cookies))

    def call(self, method, path, body=None):
        headers = {'Origin': os.environ.get('ORIGIN', self.base), 'Content-Type': 'application/json'}
        headers['X-CSRF-Token'] = next((c.value for c in self.cookies if c.name == 'csrf_token'), '')
        request = urllib.request.Request(self.base + path, method=method, headers=headers,
                                        data=json.dumps(body).encode() if body is not None else None)
        try:
            with self.opener.open(request, timeout=30) as response:
                return json.load(response).get('data')
        except urllib.error.HTTPError as error:
            raise RuntimeError(f'{method} {path}: HTTP {error.code}: {error.read().decode()}') from None

    def login(self):
        return self.call('POST', '/api/v1/auth/login', {'username': 'admin', 'password': 'AdminDemo-2026!'})

    def jobs(self, database):
        jobs, before = [], ''
        while True:
            page = self.call('GET', f'/api/v1/databases/{database}/jobs?limit=100{before}')
            jobs.extend(page['jobs'] or [])
            if not page.get('next_before'):
                return jobs
            before = f"&before={page['next_before']}"


def wait_ready(client):
    for _ in range(90):
        try:
            client.call('GET', '/health')
            return
        except (OSError, RuntimeError):
            time.sleep(2)
    raise RuntimeError('Backend not ready after 180 seconds; inspect stress backend logs')
