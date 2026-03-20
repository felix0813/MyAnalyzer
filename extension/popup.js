const statusEl = document.getElementById('status');
const historyListEl = document.getElementById('historyList');
const clearBtn = document.getElementById('clearBtn');
const endpointInputEl = document.getElementById('endpointInput');
const saveConfigBtn = document.getElementById('saveConfigBtn');
const sendBtn = document.getElementById('sendBtn');

const DISPLAY_LIMIT = 100;
const STORAGE_LIMIT = 5000;

let currentRecords = [];

function formatTime(isoString) {
  const date = new Date(isoString);
  return date.toLocaleString('zh-CN', {
    hour12: false
  });
}

function setBusy(isBusy) {
  clearBtn.disabled = isBusy;
  saveConfigBtn.disabled = isBusy;
  sendBtn.disabled = isBusy;
}

function renderRecords(records, statusMessage) {
  currentRecords = records;
  historyListEl.innerHTML = '';

  if (!records.length) {
    statusEl.textContent = statusMessage || `暂无记录。最多可在本地缓存 ${STORAGE_LIMIT} 条。`;
    const emptyItem = document.createElement('li');
    emptyItem.className = 'empty';
    emptyItem.textContent = '还没有捕获到浏览记录。';
    historyListEl.appendChild(emptyItem);
    return;
  }

  const visibleRecords = records.slice(0, DISPLAY_LIMIT);
  statusEl.textContent = statusMessage || `共 ${records.length} 条本地缓存，当前展示最近 ${visibleRecords.length} 条，最多可存储 ${STORAGE_LIMIT} 条。`;

  visibleRecords.forEach((record) => {
    const item = document.createElement('li');
    item.className = 'item';

    const title = document.createElement('div');
    title.className = 'title';
    title.textContent = record.title || '未命名页面';

    const link = document.createElement('a');
    link.className = 'url';
    link.href = record.url;
    link.target = '_blank';
    link.rel = 'noreferrer';
    link.textContent = record.url;

    const time = document.createElement('div');
    time.className = 'time';
    time.textContent = `访问时间：${formatTime(record.visitedAt)}`;

    item.append(title, link, time);
    historyListEl.appendChild(item);
  });
}

async function loadRecords() {
  statusEl.textContent = '正在加载记录...';

  const response = await chrome.runtime.sendMessage({ type: 'GET_HISTORY' });
  if (response?.error) {
    statusEl.textContent = `加载失败：${response.error}`;
    return;
  }

  endpointInputEl.value = response?.settings?.endpoint || '';
  renderRecords(response?.records || []);
}

saveConfigBtn.addEventListener('click', async () => {
  setBusy(true);
  statusEl.textContent = '正在保存地址...';

  try {
    const response = await chrome.runtime.sendMessage({
      type: 'SAVE_SETTINGS',
      endpoint: endpointInputEl.value
    });

    if (response?.error) {
      statusEl.textContent = `保存失败：${response.error}`;
      return;
    }

    endpointInputEl.value = response?.settings?.endpoint || endpointInputEl.value.trim();
    statusEl.textContent = '接收地址已保存。';
  } finally {
    setBusy(false);
  }
});

sendBtn.addEventListener('click', async () => {
  setBusy(true);
  statusEl.textContent = '正在发送数据...';

  try {
    const response = await chrome.runtime.sendMessage({
      type: 'SEND_HISTORY',
      endpoint: endpointInputEl.value
    });

    if (response?.error) {
      statusEl.textContent = `发送失败：${response.error}`;
      return;
    }

    if (!response?.skipped) {
      renderRecords([], response?.message || '发送完成。');
    } else {
      renderRecords(currentRecords, response?.message || '当前没有可发送的记录。');
    }
  } finally {
    setBusy(false);
  }
});

clearBtn.addEventListener('click', async () => {
  setBusy(true);
  try {
    const response = await chrome.runtime.sendMessage({ type: 'CLEAR_HISTORY' });
    if (response?.error) {
      statusEl.textContent = `清空失败：${response.error}`;
      return;
    }

    renderRecords([], '已清空扩展本地缓存。');
  } finally {
    setBusy(false);
  }
});

loadRecords().catch((error) => {
  statusEl.textContent = `发生错误：${error.message}`;
});
