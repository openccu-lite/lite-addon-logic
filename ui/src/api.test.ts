import {afterEach, describe, expect, it, vi} from 'vitest';

import {ApiError, apiUrl, del, get, patch, post, put, upload} from './api.js';

function answer(status: number, body: string, statusText = '') {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => new Response(body, {status, statusText}));
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe('api', () => {
    it('resolves a path against the page', () => {
        expect(apiUrl('api/v1/health')).toBe(new URL('api/v1/health', document.baseURI).toString());
    });
    it('reads JSON and sends no body on a GET', async () => {
        const f = answer(200, '{"ok":true}');
        expect(await get<{ok: boolean}>('api/v1/health')).toEqual({ok: true});
        const init = f.mock.calls[0]![1] as RequestInit;
        expect(init.method).toBeUndefined();
        expect(init.cache).toBe('no-store');
    });
    it('uploads a file as the body, with the kit\'s header and without JSON', async () => {
        const f = answer(200, '{"id":"x"}');
        const file = new Blob(['<dom/>']);
        expect(await upload<{id: string}>('api/v1/import/preview', file)).toEqual({id: 'x'});
        const init = f.mock.calls[0]![1] as RequestInit;
        expect(init.method).toBe('POST');
        expect(init.body).toBe(file);
        expect((init.headers as Record<string, string>)['X-CCU-Addon-Request']).toBe('1');
        expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/octet-stream');
    });
    it('answers an empty object for an empty answer', async () => {
        answer(200, '');
        expect(await get('x')).toEqual({});
    });
    it('sends the kit\'s header and the body on a write', async () => {
        for (const [fn, method] of [
            [post, 'POST'],
            [put, 'PUT'],
            [patch, 'PATCH'],
        ] as const) {
            const f = answer(200, '{}');
            await fn('x', {a: 1});
            const init = f.mock.calls[0]![1] as RequestInit;
            expect(init.method).toBe(method);
            expect((init.headers as Record<string, string>)['X-CCU-Addon-Request']).toBe('1');
            expect(init.body).toBe('{"a":1}');
        }
        const f = answer(200, '{}');
        await post('x');
        expect((f.mock.calls[0]![1] as RequestInit).body).toBeUndefined();
        const d = answer(200, '{}');
        await del('x');
        expect((d.mock.calls[0]![1] as RequestInit).method).toBe('DELETE');
    });
    it('turns an error answer into an ApiError with the code and the message', async () => {
        answer(422, '{"error":"invalid","message":"no"}');
        const e = (await get('x').catch((err: unknown) => err)) as ApiError;
        expect(e).toBeInstanceOf(ApiError);
        expect([e.status, e.code, e.message]).toEqual([422, 'invalid', 'no']);
        expect(e.body).toEqual({error: 'invalid', message: 'no'});
    });
    it('keeps the status of an error that is not JSON', async () => {
        answer(502, '<html>', 'Bad Gateway');
        const e = (await get('x').catch((err: unknown) => err)) as ApiError;
        expect([e.status, e.code, e.message]).toEqual([502, '502', 'Bad Gateway']);
    });
});
