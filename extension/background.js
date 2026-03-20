const STORAGE_KEY = 'recordedHistory';
const SETTINGS_KEY = 'syncSettings';
const MAX_RECORDS = 5000;
const DEFAULT_ENDPOINT = 'http://127.0.0.1:8000/api/history';

async function getRecords() {
  const result = await chrome.storage.local.get(STORAGE_KEY);
  return Array.isArray(result[STORAGE_KEY]) ? result[STORAGE_KEY] : [];
}

async function saveRecords(records) {
  await chrome.storage.local.set({ [STORAGE_KEY]: records.slice(0, MAX_RECORDS) });
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

async function appendRecord({ url, title, lastVisitTime }) {
  if (!url) {
    return;
  }

  const records = await getRecords();
  records.unshift({
    url,
    title: title || '',
    visitedAt: new Date(lastVisitTime || Date.now()).toISOString()
  });
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

  await saveRecords([]);
  return {
    success: true,
    sentCount: records.length,
    message: `已成功发送 ${records.length} 条记录，并清空本地缓存。`
  };
}

chrome.runtime.onInstalled.addListener(async () => {
  const items = await chrome.history.search({
    text: '',
    startTime: 0,
    maxResults: 100
  });

  const initialRecords = items
    .filter((item) => item.url)
    .sort((a, b) => (b.lastVisitTime || 0) - (a.lastVisitTime || 0))
    .map((item) => ({
      url: item.url,
      title: item.title || '',
      visitedAt: new Date(item.lastVisitTime || Date.now()).toISOString()
    }));

  await saveRecords(initialRecords);
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
