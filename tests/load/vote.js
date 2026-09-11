import http from 'k6/http';
import { check, sleep } from 'k6';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';
const adminToken = __ENV.ADMIN_TOKEN || 'local-admin-token-change-me';
const duplicateRatio = Number(__ENV.DUPLICATE_RATIO || '0.25');

export const options = {
  vus: Number(__ENV.VUS || '20'),
  duration: __ENV.DURATION || '20s',
  thresholds: { http_req_failed: ['rate<0.02'], http_req_duration: ['p(95)<500'] },
};

export function setup() {
  if (__ENV.POLL_ID && __ENV.OPTION_ID) return { pollID: __ENV.POLL_ID, optionID: __ENV.OPTION_ID };
  const now = Date.now();
  const body = JSON.stringify({
    question: 'Load test poll', type: 'single_choice', max_choices: 1,
    starts_at: new Date(now - 10000).toISOString(), ends_at: new Date(now + 10 * 60 * 1000).toISOString(),
    options: [{ text: 'A' }, { text: 'B' }],
  });
  const headers = { 'Content-Type': 'application/json', Authorization: `Bearer ${adminToken}` };
  const created = http.post(`${baseURL}/api/v1/admin/polls`, body, { headers });
  if (created.status !== 201) throw new Error(`create failed: ${created.status} ${created.body}`);
  const poll = created.json();
  const published = http.post(`${baseURL}/api/v1/admin/polls/${poll.id}/publish`, null, { headers });
  if (published.status !== 200) throw new Error(`publish failed: ${published.status} ${published.body}`);
  return { pollID: poll.id, optionID: poll.options[0].id };
}

export default function (data) {
  const params = { headers: { 'Content-Type': 'application/json' }, tags: { name: 'vote' } };
  const body = JSON.stringify({ option_ids: [data.optionID] });
  const response = http.post(`${baseURL}/api/v1/polls/${data.pollID}/votes`, body, params);
  check(response, { 'vote accepted': (r) => r.status === 201 || r.status === 200 });
  if (Math.random() < duplicateRatio) {
    const duplicate = http.post(`${baseURL}/api/v1/polls/${data.pollID}/votes`, body, params);
    check(duplicate, { 'duplicate is idempotent': (r) => r.status === 200 });
  }
  sleep(0.05);
}
