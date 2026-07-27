import assert from 'node:assert';
import test from 'node:test';
import { getOpenAPISpec, renderSwaggerUIHTML, openapiHandler } from '../src/middleware/openapi.js';

test('getOpenAPISpec should return valid OpenAPI 3.1 specification object', () => {
  const spec = getOpenAPISpec();
  assert.strictEqual(typeof spec, 'object');
  assert.strictEqual(spec.openapi, '3.1.0');
  assert.strictEqual(typeof spec.paths, 'object');
  assert.ok(spec.paths['/health']);
  assert.ok(spec.paths['/circles']);
});

test('renderSwaggerUIHTML should return HTML string with Embedded Swagger UI', () => {
  const html = renderSwaggerUIHTML();
  assert.strictEqual(typeof html, 'string');
  assert.ok(html.includes('swagger-ui'));
  assert.ok(html.includes('SwaggerUIBundle'));
});

test('openapiHandler should serve JSON for /api/docs/openapi.json and preserve CORS headers', () => {
  let payload = null;
  let contentType = null;
  const headers = {};
  const jsonRes = {
    type: (t) => {
      contentType = t;
      return jsonRes;
    },
    setHeader: (name, value) => {
      headers[name] = value;
    },
    send: (payloadValue) => {
      payload = payloadValue;
      return jsonRes;
    },
  };

  openapiHandler({ url: '/api/docs/openapi.json', method: 'GET' }, jsonRes);

  assert.strictEqual(payload.openapi, '3.1.0');
  assert.strictEqual(contentType, 'application/json');
  assert.strictEqual(headers['Access-Control-Allow-Origin'], '*');
});

test('openapiHandler should return HTML for /api/docs and support preflight OPTIONS', () => {
  let htmlSent = null;
  let ended = false;
  const headers = {};
  const htmlRes = {
    type: (t) => {
      return htmlRes;
    },
    setHeader: (name, value) => {
      headers[name] = value;
    },
    end: (payload) => {
      htmlSent = payload;
      ended = true;
      return htmlRes;
    },
  };

  openapiHandler({ url: '/api/docs', method: 'OPTIONS' }, htmlRes);
  assert.strictEqual(ended, true);
  assert.strictEqual(headers['Access-Control-Allow-Origin'], '*');

  openapiHandler({ url: '/api/docs', method: 'GET' }, htmlRes);
  assert.ok(htmlSent.includes('<!DOCTYPE html>'));
});
