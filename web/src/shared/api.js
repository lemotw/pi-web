async function parseJSONResponse(response) {
  let payload = null;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }
  if (!response.ok) {
    const message = payload && payload.error ? payload.error : `HTTP ${response.status}`;
    throw new Error(message);
  }
  return payload;
}

export async function getJSON(url, { fetchImpl = fetch } = {}) {
  const response = await fetchImpl(url, { headers: { Accept: 'application/json' } });
  return parseJSONResponse(response);
}

export async function postJSON(url, body, { fetchImpl = fetch } = {}) {
  const response = await fetchImpl(url, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  return parseJSONResponse(response);
}
