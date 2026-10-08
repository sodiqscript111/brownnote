export class ServiceError extends Error {
  constructor(message, retryable = false) {
    super(message); this.name = 'ServiceError'; this.retryable = retryable;
  }
}

const aborted = () => new DOMException('Request cancelled', 'AbortError');

export function abortableSleep(ms, signal) {
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(aborted()); return; }
    const done = () => { signal.removeEventListener('abort', cancel); resolve(); };
    const timer = setTimeout(done, ms);
    const cancel = () => { clearTimeout(timer); signal.removeEventListener('abort', cancel); reject(aborted()); };
    signal.addEventListener('abort', cancel, { once: true });
  });
}
export async function attemptJSON(fetcher, url, body, signal, timeoutMs) {
  const attempt = new AbortController();
  let timer, cancel;
  const stopped = new Promise((_, reject) => {
    cancel = () => { reject(aborted()); attempt.abort(); };
    if (signal.aborted) { cancel(); return; }
    signal.addEventListener('abort', cancel, { once: true });
    timer = setTimeout(() => {
      reject(new ServiceError('Spelling service timed out. Your text is safe; edit to retry.', true)); attempt.abort();
    }, timeoutMs);
  });
  const request = async () => {
    if (signal.aborted) throw aborted();
    let response;
    try {
      response = await fetcher(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body, signal: attempt.signal });
    } catch (error) {
      if (signal.aborted || attempt.signal.aborted) throw aborted();
      if (error instanceof TypeError) throw new ServiceError('Spelling service unreachable. Your text is safe; edit to retry.', true);
      throw error;
    }
    if (!response.ok) {
      const retryable = [502, 503, 504].includes(response.status);
      if (response.body?.cancel) await response.body.cancel().catch(() => {});
      throw new ServiceError('Spelling service unavailable. Your text is safe; edit to retry.', retryable);
    }
    try { return await response.json(); }
    catch (error) {
      if (signal.aborted || attempt.signal.aborted) throw aborted();
      throw new ServiceError('Invalid spelling response');
    }
  };
  try { return await Promise.race([stopped, request()]); }
  finally { clearTimeout(timer); signal.removeEventListener('abort', cancel); }
}

export class CircuitBreaker {
  constructor({ threshold = 3, cooldownMs = 30000, now = () => Date.now() } = {}) {
    if (!Number.isInteger(threshold) || threshold < 1 || !Number.isFinite(cooldownMs) || cooldownMs < 0) throw new Error('Invalid circuit settings');
    this.threshold = threshold; this.cooldownMs = cooldownMs; this.now = now;
    this.failures = 0; this.state = 'closed'; this.openUntil = 0;
  }
  begin() {
    if (this.state === 'half-open') throw new ServiceError('Spelling service recovery check in progress. Your text is safe.');
    if (this.state === 'open') {
      if (this.now() < this.openUntil) {
        const seconds = Math.ceil((this.openUntil - this.now()) / 1000);
        throw new ServiceError(`Spelling paused. Try another edit in ${seconds}s; your text is safe.`);
      }
      this.state = 'half-open'; this.probe = { probe: true }; return this.probe;
    }
    return { probe: false };
  }
  success() { this.failures = 0; this.state = 'closed'; this.openUntil = 0; this.probe = null; }
  failure(permit) {
    if (permit.probe || ++this.failures >= this.threshold) {
      this.state = 'open'; this.openUntil = this.now() + this.cooldownMs;
      this.probe = null;
    }
  }
  cancel(permit) {
    if (permit === this.probe && this.state === 'half-open') { this.state = 'open'; this.probe = null; }
  }
}
