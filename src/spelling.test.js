import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { LRUCache, SpellClient, tokenize, normalize, decorations, correction, debouncer } from './spelling.js';

const good = word => ({ word, correct: true, suggestions: [] });
const bad = word => ({ word, correct: false, suggestions: ['hello'] });
const response = results => ({ ok: true, json: async () => ({ results }) });

test('debouncing waits 500ms after the last edit and cancellation suppresses checking', context => {
  context.mock.timers.enable({ apis: ['setTimeout'] });
  const calls = []; const checking = debouncer(text => calls.push(text));
  checking.schedule('h'); context.mock.timers.tick(400); assert.deepEqual(calls,[]);
  checking.schedule('hello'); context.mock.timers.tick(499); assert.deepEqual(calls,[]);
  context.mock.timers.tick(1); assert.deepEqual(calls,['hello']);
  checking.schedule('hello world'); checking.cancel(); context.mock.timers.tick(1000);
  assert.deepEqual(calls,['hello']);
});

test('LRU capacity is 512; reads refresh recency and writes replace entries', () => {
  const cache = new LRUCache();
  for (let i = 0; i < 512; i++) cache.set(String(i), good(String(i)));
  cache.get('0'); cache.set('512', good('512'));
  assert.equal(cache.size, 512); assert.equal(cache.get('1'), undefined);
  assert.ok(cache.get('0')); cache.set('0', bad('0'));
  assert.equal(cache.size, 512); assert.equal(cache.get('0').correct, false);
});

test('token positions retain contractions, punctuation, repeated words, Unicode and casing', () => {
  const text = '🙂 HELLO, hello! Don’t re-check café. Cafe\u0301';
  const tokens = tokenize(text);
  assert.deepEqual(tokens.map(t => t.key), ['hello','hello',"don't",'re','check','café','café']);
  for (const t of tokens) assert.equal(text.slice(t.start,t.end),t.word);
  assert.equal(tokens[0].start,3);
  assert.equal(normalize('DON’T'),"don't");
});

test('a single batch deduplicates words; one added word only requests that word', async () => {
  const calls = [];
  const client = new SpellClient({ fetcher: async (_, options) => {
    const words = JSON.parse(options.body).words; calls.push(words);
    return response(words.map(w => w === 'helo' ? bad(w) : good(w)));
  }});
  const first = await client.check('Hello, HELO! helo.');
  assert.deepEqual(calls,[['hello','helo']]);
  assert.equal(decorations('Hello, HELO! helo.',first.results).length,2);
  await client.check('Hello, HELO! helo. World');
  assert.deepEqual(calls,[['hello','helo'],['world']]);
  await client.check('WORLD! Hello helo'); assert.equal(calls.length,2);
});

test('corrections target one occurrence, preserve punctuation/case and adjust selection', () => {
  const text = 'Helo, helo!'; const tokens = tokenize(text);
  const next = correction(text,tokens[1],'hello',10,10);
  assert.equal(next.text,'Helo, hello!'); assert.equal(next.start,11);
  assert.equal(correction(text,tokens[0],'hello',0,0).text,'Hello, helo!');
  assert.equal(correction('HELO',tokenize('HELO')[0],'hello',4,4).text,'HELLO');
  assert.equal(correction('DON’T',tokenize('DON’T')[0],"don't",5,5).text,'DON’T');
  assert.equal(correction('changed',tokens[1],'hello',0,0),null);
  const selected = correction(text,tokens[0],'hello',6,10);
  assert.deepEqual([selected.start,selected.end],[7,11]);
});

test('insertions and deletions reconstruct positions from current text', () => {
  const results = new Map([['helo',bad('helo')]]);
  assert.deepEqual(decorations('hello! helo, helo.',results).map(t => t.start),[7,13]);
  assert.deepEqual(decorations('helo.',results).map(t => t.start),[0]);
  assert.equal(decorations('',results).length,0);
  assert.equal(decorations('hello',results).length,0);
});

test('stale responses and failures cannot populate the cache or update newer text', async () => {
  const pending=[];
  const client=new SpellClient({fetcher:(_,options)=>new Promise(resolve=>pending.push({resolve,options}))});
  const old=client.check('helo'); const fresh=client.check('hello');
  assert.equal(pending[0].options.signal.aborted,true);
  pending[1].resolve(response([good('hello')])); assert.ok(await fresh);
  pending[0].resolve(response([bad('helo')])); assert.equal(await old,null);
  assert.equal(client.cache.get('helo'),undefined);
  const cancelled=client.check('word'); client.cancel();
  pending[2].resolve(response([good('word')])); assert.equal(await cancelled,null);
});

test('malformed and failed responses do not poison cached words', async () => {
  const client=new SpellClient({fetcher:async()=>response([good('hello'),good('wrong')])});
  await assert.rejects(client.check('hello world'),/Invalid spelling response/);
  assert.equal(client.cache.size,0);
  client.fetcher=async()=>({ok:false});
  await assert.rejects(client.check('hello'),/unavailable/);
  assert.equal(client.cache.size,0);
});

test('evicted words are requested again; cached words never regenerate suggestions', async () => {
  const calls=[]; const client=new SpellClient({cache:new LRUCache(2),fetcher:async(_,o)=>{
    const words=JSON.parse(o.body).words; calls.push(words); return response(words.map(good));
  }});
  await client.check('one two'); await client.check('one'); await client.check('three');
  await client.check('one two'); assert.deepEqual(calls,[['one','two'],['three'],['two']]);
});

test('native fetch keeps its browser/global receiver and calls HTTP only on cache misses', async () => {
  let count = 0;
  const server = createServer((req, res) => {
    count++; let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => { res.setHeader('Content-Type','application/json'); res.end(JSON.stringify({results:JSON.parse(body).words.map(good)})); });
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  try {
    const client = new SpellClient({url:`http://127.0.0.1:${server.address().port}/api/check`});
    await client.check('hello hello'); await client.check('HELLO!');
    assert.equal(count,1);
  } finally { await new Promise(resolve => server.close(resolve)); }
});

test('empty, very long and excessively large batches never reach the API', async () => {
  let calls=0;const client=new SpellClient({fetcher:async()=>{calls++;throw new Error('unexpected request')}});
  await client.check('');await client.check('123 — '+ 'a'.repeat(65));assert.equal(calls,0);
  const text=Array.from({length:4097},(_,i)=>'word'+i.toString(16).replace(/[0-9a-f]/g,c=>String.fromCharCode(97+parseInt(c,16)))).join(' ');
  const keys=[...new Set(tokenize(text).map(t=>t.key))];
  assert.equal(keys.length,4097);
  await assert.rejects(client.check(text),/limit/);
  assert.equal(calls,0);
});
