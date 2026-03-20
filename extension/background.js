const STORAGE_KEY = 'recordedHistory';
const MAX_RECORDS = 5000;

async function getRecords() {
  const result = await chrome.storage.local.get(STORAGE_KEY);
  return Array.isArray(result[STORAGE_KEY]) ? result[STORAGE_KEY] : [];
}

async function saveRecords(records) {
  await chrome.storage.local.set({ [STORAGE_KEY]: records.slice(0, MAX_RECORDS) });
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
});

chrome.history.onVisited.addListener(async (historyItem) => {
  await appendRecord(historyItem);
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === 'GET_HISTORY') {
    getRecords()
      .then((records) => sendResponse({ records }))
      .catch((error) => sendResponse({ error: error.message }));
    return true;
  }

  if (message?.type === 'CLEAR_HISTORY') {
    saveRecords([])
      .then(() => sendResponse({ success: true }))
      .catch((error) => sendResponse({ error: error.message }));
    return true;
  }

  return false;
});
