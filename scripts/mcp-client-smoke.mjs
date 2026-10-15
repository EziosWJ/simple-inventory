// Optional compatibility smoke using the official JS SDK declared by Cherry 1.6.0.
// npm install --prefix /tmp/mcp-js-client --ignore-scripts @modelcontextprotocol/sdk@1.17.5
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import assert from 'node:assert/strict';
const require = createRequire(resolve(process.env.MCP_SDK_DIR || '/tmp/mcp-js-client', 'package.json'));
const { Client } = require('@modelcontextprotocol/sdk/client/index.js');
const { StreamableHTTPClientTransport } = require('@modelcontextprotocol/sdk/client/streamableHttp.js');
const credentialFile = process.argv[2];
if (!credentialFile) throw new Error('usage: node scripts/mcp-client-smoke.mjs <private-credential.json> [mcp-url]');
const endpoint = new URL(process.argv[3] || 'http://127.0.0.1:18300/mcp');
assert(['http:', 'https:'].includes(endpoint.protocol));
const { token } = JSON.parse(readFileSync(credentialFile, 'utf8'));
const observed = [];
const transport = new StreamableHTTPClientTransport(endpoint, {
  requestInit: { headers: { Authorization: `Bearer ${token}` } },
  fetch: async (url, init) => {
    const headers = new Headers(init.headers);
    observed.push({ method: init.method, authorization: headers.get('authorization') === `Bearer ${token}`,
      accept: headers.get('accept'), protocol: headers.get('mcp-protocol-version') });
    return fetch(url, init);
  }
});
const client = new Client({ name: 'cherry-compatible-smoke', version: '1.17.5' }, { capabilities: {} });
await client.connect(transport);
const tools = await client.listTools();
assert.equal(tools.tools.length, 4);
async function call(name, args) {
  const result = await client.callTool({ name, arguments: args });
  assert(!result.isError, JSON.stringify(result));
  return JSON.parse(result.content[0].text);
}
const candidates = await call('search_products', { keyword: '同名硒鼓' });
assert.equal(candidates.items.length, 2);
assert.equal(candidates.selectionRequired, true);
// Deterministically choose the explicit fixture code, never the first match.
const chosen = candidates.items.find(item => item.code === 'DEMO-G1');
assert(chosen);
const stock = await call('get_inventory_balances', { productId: chosen.id });
assert.equal(stock.quantity, '12.5');
const partners = await call('search_partners', { keyword: '双向往来样例' });
assert.equal(partners.items.length, 1);
const balances = await call('get_partner_balances', { partnerId: partners.items[0].id });
assert.equal(balances.items.find(x => x.direction === 'CUSTOMER').amount, '123.45');
assert.equal(balances.items.find(x => x.direction === 'SUPPLIER').amount, '-67.89');
await client.close();
assert(observed.every(x => x.authorization));
assert(observed.some(x => x.protocol === '2025-06-18'));
const bad = await fetch(endpoint, { method: 'POST', headers: { Authorization: 'Bearer invalid',
  'Content-Type': 'application/json', Accept: 'application/json, text/event-stream' },
  body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/list' }) });
assert.equal(bad.status, 401);
console.log(JSON.stringify({ sdk: '1.17.5', passed: ['connect', 'four-tools', 'candidate-selection', 'stock',
  'signed-dual-balances', 'bad-token'], requests: observed }, null, 2));
