import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter, Gauge } from 'k6/metrics';

const fixture = JSON.parse(open('/results/fixture.json'));
const base = __ENV.BASE_URL || 'http://backend:8080';
const hold = fixture.hold;
const ramp = fixture.ramp;
const total = 4 * hold + 3 * ramp;
const maxVUs = Number(__ENV.MAX_VUS || 20);
const origin = new Gauge('run_origin_ms');
const exhausted = new Counter('fixture_exhausted');
const completed = new Counter('observed_job_success');
const stages = (rate) => [
  { duration: `${hold}s`, target: rate },
  { duration: `${ramp}s`, target: 2 * rate },
  { duration: `${hold}s`, target: 2 * rate },
  { duration: `${ramp}s`, target: 4 * rate },
  { duration: `${hold}s`, target: 4 * rate },
  { duration: `${ramp}s`, target: rate },
  { duration: `${hold}s`, target: rate },
];
const scenario = (fn, rate, unit) => ({
  executor: 'ramping-arrival-rate', exec: fn, startRate: rate, timeUnit: unit,
  preAllocatedVUs: maxVUs, maxVUs, stages: stages(rate),
  gracefulStop: `${Number(__ENV.DRAIN_SECONDS || 300)}s`,
});
export const options = {
  noCookiesReset: true, // Keep each VU session across iterations.
  scenarios: {
    reads: scenario('readAPI', Number(__ENV.API_RATE || 10), '1s'),
    migrations: scenario('submitJob', Number(__ENV.JOB_RATE || 12), '1m'),
  },
  thresholds: {
    'http_req_duration{kind:api}': [`p(95)<${Number(__ENV.API_P95_MS || 500)}`],
    'http_req_failed{kind:api}': [`rate<${Number(__ENV.ERROR_RATE || 0.01)}`],
    fixture_exhausted: ['count==0'],
    dropped_iterations: ['count==0'],
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'max'],
};

export function setup() {
  exhausted.add(0);
  return {};
}
let loggedIn = false;
function phase(start) {
  const t = (Date.now() - start) / 1000;
  if (t < hold) return 'normal';
  if (t < hold + ramp) return 'ramp_2x';
  if (t < 2 * hold + ramp) return '2x';
  if (t < 2 * hold + 2 * ramp) return 'ramp_4x';
  if (t < 3 * hold + 2 * ramp) return '4x';
  if (t < 3 * hold + 3 * ramp) return 'ramp_down';
  return t < total ? 'recovery' : 'drain';
}
function params(kind, stage, name) {
  const token = (http.cookieJar().cookiesForURL(base).csrf_token || [])[0];
  return {
    headers: { Origin: __ENV.ORIGIN || base, 'Content-Type': 'application/json', 'X-CSRF-Token': token || '' },
    tags: { kind, phase: stage, name }, timeout: '15s',
  };
}
function login() {
  if (loggedIn) return;
  const r = http.post(`${base}/api/v1/auth/login`, JSON.stringify({ username: 'admin', password: 'AdminDemo-2026!' }), params('auth', 'auth', 'login'));
  if (!check(r, { 'login succeeds': (r) => r.status === 200 })) throw new Error('VU login failed');
  loggedIn = true;
}
export function readAPI(data) {
  login();
  const index = exec.scenario.iterationInTest;
  if (index === 0) origin.add(exec.scenario.startTime);
  const db = fixture.databases[index % fixture.databases.length];
  const path = index % 2 === 0 ? 'schemas' : `schemas/public/tables/${fixture.table}/rows?limit=20`;
  const r = http.get(`${base}/api/v1/databases/${db}/${path}`, params('api', phase(exec.scenario.startTime), index % 2 === 0 ? 'schemas' : 'rows'));
  check(r, { 'read succeeds': (r) => r.status === 200 });
}
export function submitJob(data) {
  login();
  const f = fixture.jobs[exec.scenario.iterationInTest];
  if (!f) { exhausted.add(1); return; }
  const r = http.post(`${base}/api/v1/databases/${f.database}/migrations/${f.migration}/execute`, '{}', params('api', phase(exec.scenario.startTime), 'execute'));
  if (!check(r, { 'submission accepted': (r) => r.status === 200 })) return;
  let receipt;
  try { receipt = r.json('data'); } catch (_) { return; }
  if (!receipt || !receipt.job_id) return;
  const deadline = Date.now() + Number(__ENV.DRAIN_SECONDS || 300) * 1000;
  while (Date.now() < deadline) {
    sleep(1);
    const p = http.get(`${base}/api/v1/databases/${f.database}/jobs/${receipt.job_id}`, params('poll', phase(exec.scenario.startTime), 'job-status'));
    if (p.status !== 200) continue;
    const job = p.json('data');
    if (job.status === 'SUCCEEDED') { completed.add(1); return; }
    if (['FAILED', 'BLOCKED'].includes(job.status)) return;
  }
}
export function handleSummary(data) {
  return { '/results/summary.json': JSON.stringify(data, null, 2), stdout: JSON.stringify(data.metrics, null, 2) + '\n' };
}
