import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';

const source = await readFile(new URL('../src/lib/datetime.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText;
const datetime = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);
const ledgerSource = await readFile(new URL('../src/lib/partner-ledger.ts', import.meta.url), 'utf8');
const ledgerJs = ts.transpileModule(ledgerSource, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
  .replace('"@/lib/datetime"', JSON.stringify(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`));
const ledger = await import(`data:text/javascript;base64,${Buffer.from(ledgerJs).toString('base64')}`);
for (const zone of ['UTC', 'America/Los_Angeles', 'Asia/Shanghai']) {
  process.env.TZ = zone;
  assert.equal(datetime.formatDateTime('2026-10-09T16:15:00Z'), '2026-10-10 00:15:00');
  assert.equal(datetime.formatDateTime('2026-10-10T00:15:00+08:00'), '2026-10-10 00:15:00');
  assert.equal(datetime.formatDateTime('2026-10-09 16:15:00'), '2026-10-10 00:15:00');
  assert.equal(datetime.formatDateOnly('2026-10-10'), '2026-10-10');
  assert.equal(datetime.formatDateOnly('2026-10-09T16:15:00Z'), '2026-10-10');
  assert.equal(datetime.formatDateTime(''), '-');
  assert.equal(datetime.formatDateTime('invalid'), '-');
  assert.equal(datetime.businessToday(new Date('2026-10-09T16:15:00Z')), '2026-10-10');
  assert.equal(datetime.businessMidnightToUtc('2026-10-10'), '2026-10-09T16:00:00.000Z');
  assert.equal(datetime.businessMidnightToUtc('2026-12-31', true), '2026-12-31T16:00:00.000Z');
  assert.deepEqual(datetime.businessMonthRange(new Date('2026-10-09T16:15:00Z')), { from: '2026-10-01', to: '2026-10-31' });
  assert.deepEqual(datetime.businessMonthRange(new Date('2026-10-31T16:30:00Z')), { from: '2026-11-01', to: '2026-11-30' });
  assert.deepEqual(datetime.businessMonthRange(new Date('2028-02-15T00:00:00Z')), { from: '2028-02-01', to: '2028-02-29' });
  assert.deepEqual(datetime.businessMonthRange(new Date('2026-12-31T20:00:00Z')), { from: '2027-01-01', to: '2027-01-31' });
  assert.deepEqual(ledger.localPeriod('2026-10-10', '2026-10-10'), { from: '2026-10-09T16:00:00.000Z', to: '2026-10-10T16:00:00.000Z' });
  assert.equal(ledger.periodDateInput('2026-10-09T16:00:00Z'), '2026-10-10');
  assert.equal(ledger.periodDateInput('2026-10-10T16:00:00Z', true), '2026-10-10');
  assert.throws(() => ledger.localPeriod('2026-10-11', '2026-10-10'), /期间开始不能晚于结束/);
}
console.log('PASS Beijing display, business dates, business month range and inclusive/exclusive day boundaries in three host timezones');
