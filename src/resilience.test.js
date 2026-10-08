import { test } from 'node:test';
import assert from 'node:assert/strict';
import { SpellClient } from './spelling.js';
import { CircuitBreaker, abortableSleep } from './resilience.js';

const result = word => ({word, correct:true, suggestions:[]});
const ok = word => ({ok:true,json:async()=>({results:[result(word)]})});
const failed = status => ({ok:false,status});
const options = {random:()=>0.5,sleep:async()=>{}};
const flush = async () => { for(let i=0;i<15;i++) await Promise.resolve(); };

test('network and gateway failures retry twice after 1s and 2s, caching final success', async () => {
  for(const failure of [new TypeError('offline'),502,503,504]) {
    let calls=0;const delays=[];
    const client=new SpellClient({...options,sleep:async ms=>delays.push(ms),fetcher:async()=>{
      calls++;if(calls<3){if(failure instanceof Error)throw failure;return failed(failure)}return ok('hello');
    }});
    assert.ok(await client.check('hello'));
    assert.deepEqual(delays,[1000,2000]);assert.equal(calls,3);
    assert.equal(client.breaker.failures,0);await client.check('HELLO');assert.equal(calls,3);
  }
});

test('invalid requests and malformed responses are not retried or counted as outages', async () => {
  for(const status of [400,401,403,404,413,415,500]) {
    let calls=0;const client=new SpellClient({...options,fetcher:async()=>{calls++;return failed(status)}});
    await assert.rejects(client.check('hello'),/unavailable/);assert.equal(calls,1);assert.equal(client.breaker.failures,0);
  }
  let calls=0;const client=new SpellClient({...options,fetcher:async()=>{calls++;return {ok:true,json:async()=>({results:[]})}}});
  await assert.rejects(client.check('hello'),/Invalid/);assert.equal(calls,1);assert.equal(client.cache.size,0);
});

test('three exhausted checks open the circuit; cache still works; cooldown permits one probe', async () => {
  let time=0,calls=0,healthy=false;
  const breaker=new CircuitBreaker({now:()=>time});
  const client=new SpellClient({...options,breaker,fetcher:async()=>{calls++;return healthy?ok('hello'):failed(503)}});
  client.cache.set('cached',result('cached'));
  for(let i=0;i<3;i++) await assert.rejects(client.check('hello'));
  assert.equal(calls,9);assert.equal(breaker.failures,3);assert.equal(breaker.state,'open');
  await assert.rejects(client.check('hello'),/paused/);assert.equal(calls,9);
  assert.ok(await client.check('cached'));assert.equal(calls,9);assert.equal(breaker.state,'open');
  time=29999;await assert.rejects(client.check('hello'),/paused/);assert.equal(calls,9);
  time=30000;await assert.rejects(client.check('hello'),/paused/);assert.equal(calls,10);assert.equal(breaker.openUntil,60000);
  time=60000;healthy=true;assert.ok(await client.check('hello'));assert.equal(calls,11);
  assert.equal(breaker.state,'closed');assert.equal(breaker.failures,0);
});

test('successful service response resets consecutive failures', async()=>{
  let healthy=false;const client=new SpellClient({...options,retryDelays:[],fetcher:async()=>healthy?ok('hello'):failed(503)});
  await assert.rejects(client.check('hello'));await assert.rejects(client.check('hello'));assert.equal(client.breaker.failures,2);
  healthy=true;await client.check('hello');assert.equal(client.breaker.failures,0);
});

test('an edit cancels retry delays without sending more requests or counting a failure', async () => {
  let calls=0,waiting=false;
  const client=new SpellClient({...options,sleep:(ms,signal)=>{waiting=true;return abortableSleep(ms,signal)},fetcher:async()=>{calls++;return failed(503)}});
  const pending=client.check('hello');await flush();assert.equal(waiting,true);
  client.cancel();assert.equal(await pending,null);assert.equal(calls,1);assert.equal(client.breaker.failures,0);
});

test('timeout settles even when fetch or its response body ignores cancellation', async () => {
  for(const bodyHang of [false,true]) {
    let signal;
    const client=new SpellClient({...options,retryDelays:[],timeoutMs:5,fetcher:async(_,o)=>{
      signal=o.signal;return bodyHang?{ok:true,json:()=>new Promise(()=>{})}:new Promise(()=>{});
    }});
    await assert.rejects(client.check('hello'),/timed out/);assert.equal(signal.aborted,true);assert.equal(client.breaker.failures,1);
  }
});

test('cancelling a request settles it even when the transport ignores abort', async()=>{
  const client=new SpellClient({...options,fetcher:()=>new Promise(()=>{})});
  const pending=client.check('hello');client.cancel();assert.equal(await pending,null);assert.equal(client.breaker.failures,0);
});

test('only one recovery probe is admitted; cancelling an old probe cannot cancel its replacement', async()=>{
  let time=0;const breaker=new CircuitBreaker({threshold:1,now:()=>time});
  breaker.failure(breaker.begin());time=30000;
  const old=breaker.begin();assert.throws(()=>breaker.begin(),/in progress/);
  breaker.cancel(old);const replacement=breaker.begin();breaker.cancel(old);
  assert.equal(breaker.state,'half-open');breaker.cancel(replacement);assert.equal(breaker.state,'open');assert.equal(breaker.failures,1);
});

test('default delays receive bounded jitter',async()=>{
  for(const random of [0,1]) {
    const delays=[];const client=new SpellClient({random:()=>random,sleep:async ms=>delays.push(ms),fetcher:async()=>failed(503)});
    await assert.rejects(client.check('hello'));assert.deepEqual(delays,random===0?[800,1600]:[1200,2400]);
  }
});
