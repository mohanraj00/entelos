// The client of the HTTP approval API (src/protocol/approval/APPROVAL.md). The page has no state of its
// own on a server: each change goes through this call. The token stays in the memory of the page.

export const PATH = '/v1/request';

let counter = 0;

// call sends one request object and gives the response object. The response object is the record;
// the HTTP status is only a hint. A network failure or a body that is not JSON gives a local failure.
export async function call(token, operation, body) {
  counter += 1;
  const request = { protocol: '1.1', request_id: `dashboard-${counter}`, operation, body };
  let reply;
  try {
    reply = await fetch(PATH, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(request),
      cache: 'no-store',
      credentials: 'omit',
      redirect: 'error',
    });
  } catch (error) {
    return local('network_error', `The API did not answer: ${error}`);
  }
  let response;
  try {
    response = await reply.json();
  } catch {
    return local('invalid_response', `The API sent a body that is not JSON (HTTP ${reply.status}).`);
  }
  if (response === null || typeof response !== 'object' || typeof response.status !== 'string') {
    return local('invalid_response', `The API sent a body that is not a response object (HTTP ${reply.status}).`);
  }
  return response;
}

function local(code, message) {
  return { status: code, diagnostic: { code, stage: 'client', message } };
}
