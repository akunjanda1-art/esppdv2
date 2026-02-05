import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 50,
  duration: '1m',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<200'],
  },
};

const BASE = __ENV.BASE_URL || 'http://localhost:8000'; // Kong proxy by default
const USER = __ENV.USERNAME || 'admin';
const PASS = __ENV.PASSWORD || 'admin123';

function login() {
  const res = http.post(`${BASE}/v1/auth/login`, JSON.stringify({ username: USER, password: PASS }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(res, { 'login 200': (r) => r.status === 200 });
  const body = res.json();
  return body.access_token;
}

export default function () {
  const token = login();

  const nomor = `SPD-${__VU}-${__ITER}-${Date.now()}`;

  const create = http.post(
    `${BASE}/v1/spds/`,
    JSON.stringify({ nomor_surat: nomor, unit_id: 1, purpose: 'Kegiatan dinas', total_cost: '1000000' }),
    { headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` } }
  );
  check(create, { 'create 201': (r) => r.status === 201 });

  const spd = create.json();

  const submit = http.post(
    `${BASE}/v1/spds/${spd.id}/submit`,
    null,
    { headers: { Authorization: `Bearer ${token}` } }
  );
  check(submit, { 'submit 200': (r) => r.status === 200 });

  sleep(1);
}
