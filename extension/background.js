const STORAGE_KEY = 'recordedHistory';
const SETTINGS_KEY = 'syncSettings';
const SYNC_STATE_KEY = 'syncState';
const MAX_RECORDS = 5000;
const DEFAULT_ENDPOINT = 'http://127.0.0.1:8000/api/history';

function normalizeTimestamp(value) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }

  if (typeof value === 'string') {
    const parsed = Date.parse(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }

  return null;
}

function buildRecord({ url, title, lastVisitTime, visitedAt }) {
  const normalizedVisitedAt = normalizeTimestamp(lastVisitTime ?? visitedAt) ?? Date.now();

  return {
    url,
    title: title || '',
    visitedAt: new Date(normalizedVisitedAt).toISOString()
  };
}

function sortRecordsByVisitedAtDesc(records) {
  return [...records].sort(
    (a, b) => (normalizeTimestamp(b.visitedAt) || 0) - (normalizeTimestamp(a.visitedAt) || 0)
  );
}

function dedupeRecords(records) {
  const seen = new Set();
  const uniqueRecords = [];

  sortRecordsByVisitedAtDesc(records).forEach((record) => {
    const key = `${record.url}::${record.visitedAt}`;
    if (!record.url || seen.has(key)) {
      return;
    }

    seen.add(key);
    uniqueRecords.push(record);
  });

  return uniqueRecords;
}

function getLatestVisitedAt(records) {
  return records.reduce((latest, record) => {
    const visitedAt = normalizeTimestamp(record.visitedAt);
    return visitedAt && visitedAt > latest ? visitedAt : latest;
  }, 0);
}

async function getRecords() {
  const result = await chrome.storage.local.get(STORAGE_KEY);
  return Array.isArray(result[STORAGE_KEY]) ? result[STORAGE_KEY] : [];
}

async function saveRecords(records) {
  await chrome.storage.local.set({ [STORAGE_KEY]: dedupeRecords(records).slice(0, MAX_RECORDS) });
}

async function getSettings() {
  const result = await chrome.storage.local.get(SETTINGS_KEY);
  return {
    endpoint: result[SETTINGS_KEY]?.endpoint || DEFAULT_ENDPOINT
  };
}

async function saveSettings(settings) {
  const currentSettings = await getSettings();
  const nextSettings = {
    ...currentSettings,
    ...settings,
    endpoint: (settings.endpoint || currentSettings.endpoint || DEFAULT_ENDPOINT).trim()
  };

  await chrome.storage.local.set({ [SETTINGS_KEY]: nextSettings });
  return nextSettings;
}

async function getSyncState() {
  const result = await chrome.storage.local.get(SYNC_STATE_KEY);
  const lastSyncedAt = normalizeTimestamp(result[SYNC_STATE_KEY]?.lastSyncedAt);

  return {
    lastSyncedAt: lastSyncedAt ? new Date(lastSyncedAt).toISOString() : null
  };
}

async function saveSyncState(syncState) {
  const currentSyncState = await getSyncState();
  const normalizedLastSyncedAt = normalizeTimestamp(syncState.lastSyncedAt || currentSyncState.lastSyncedAt);
  const nextSyncState = {
    ...currentSyncState,
    ...syncState,
    lastSyncedAt: normalizedLastSyncedAt ? new Date(normalizedLastSyncedAt).toISOString() : null
  };

  await chrome.storage.local.set({ [SYNC_STATE_KEY]: nextSyncState });
  return nextSyncState;
}

async function appendRecord({ url, title, lastVisitTime }) {
  if (!url) {
    return;
  }

  const normalizedLastVisitTime = normalizeTimestamp(lastVisitTime) ?? Date.now();
  const { lastSyncedAt } = await getSyncState();
  const normalizedLastSyncedAt = normalizeTimestamp(lastSyncedAt);

  if (normalizedLastSyncedAt && normalizedLastVisitTime <= normalizedLastSyncedAt) {
    return;
  }

  const records = await getRecords();
  records.unshift(buildRecord({ url, title, lastVisitTime: normalizedLastVisitTime }));
  await saveRecords(records);
}

async function sendRecordsToEndpoint(endpoint) {
  const trimmedEndpoint = (endpoint || '').trim();
  if (!trimmedEndpoint) {
    throw new Error('请先填写接收数据的本地 Agent 或后端地址。');
  }

  const records = await getRecords();
  if (!records.length) {
    return {
      success: true,
      skipped: true,
      message: '当前没有可发送的记录。',
      sentCount: 0
    };
  }

  let response;
  try {
    response = await fetch(trimmedEndpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        source: 'edge-history-recorder',
        sentAt: new Date().toISOString(),
        recordCount: records.length,
        records
      })
    });
  } catch (error) {
    throw new Error(`无法连接到 ${trimmedEndpoint}：${error.message}`);
  }

  if (!response.ok) {
    const responseText = await response.text();
    throw new Error(`发送失败（HTTP ${response.status}）${responseText ? `：${responseText}` : ''}`);
  }

  const latestVisitedAt = getLatestVisitedAt(records);
  await Promise.all([
    saveRecords([]),
    saveSyncState({ lastSyncedAt: latestVisitedAt ? new Date(latestVisitedAt).toISOString() : null })
  ]);

  return {
    success: true,
    sentCount: records.length,
    message: `已成功发送 ${records.length} 条记录，并清空本地缓存。`
  };
}

chrome.runtime.onInstalled.addListener(async () => {
  const [cachedRecords, syncState] = await Promise.all([getRecords(), getSyncState()]);
  const normalizedLastSyncedAt = normalizeTimestamp(syncState.lastSyncedAt);
  const items = await chrome.history.search({
    text: '',
    startTime: normalizedLastSyncedAt ? normalizedLastSyncedAt + 1 : 0,
    maxResults: MAX_RECORDS
  });

  const initialRecords = items
    .filter((item) => item.url)
    .map((item) => buildRecord(item))
    .filter((item) => {
      const visitedAt = normalizeTimestamp(item.visitedAt);
      return !normalizedLastSyncedAt || (visitedAt && visitedAt > normalizedLastSyncedAt);
    });

  await saveRecords([...cachedRecords, ...initialRecords]);
  await saveSettings({ endpoint: DEFAULT_ENDPOINT });
});

chrome.history.onVisited.addListener(async (historyItem) => {
  await appendRecord(historyItem);
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === 'GET_HISTORY') {
    Promise.all([getRecords(), getSettings()])
      .then(([records, settings]) => sendResponse({ records, settings }))
      .catch((error) => sendResponse({ error: error.message }));
    return true;
  }

  if (message?.type === 'CLEAR_HISTORY') {
    saveRecords([])
      .then(() => sendResponse({ success: true }))
      .catch((error) => sendResponse({ error: error.message }));
    return true;
  }

  if (message?.type === 'SAVE_SETTINGS') {
    saveSettings({ endpoint: message.endpoint || '' })
      .then((settings) => sendResponse({ success: true, settings }))
      .catch((error) => sendResponse({ error: error.message }));
    return true;
  }

  if (message?.type === 'SEND_HISTORY') {
    sendRecordsToEndpoint(message.endpoint)
      .then((result) => sendResponse(result))
      .catch((error) => sendResponse({ error: error.message }));
    return true;
  }

  return false;
});
