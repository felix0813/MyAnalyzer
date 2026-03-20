const statusEl = document.getElementById('status');
const historyListEl = document.getElementById('historyList');
const clearBtn = document.getElementById('clearBtn');

function formatTime(isoString) {
  const date = new Date(isoString);
  return date.toLocaleString('zh-CN', {
    hour12: false
  });
}

function renderRecords(records) {
  historyListEl.innerHTML = '';

  if (!records.length) {
    statusEl.textContent = '暂无记录。';
    const emptyItem = document.createElement('li');
    emptyItem.className = 'empty';
    emptyItem.textContent = '还没有捕获到浏览记录。';
    historyListEl.appendChild(emptyItem);
    return;
  }

  statusEl.textContent = `共 ${records.length} 条记录（显示本地缓存）`;

  records.forEach((record) => {
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

  renderRecords(response?.records || []);
}

clearBtn.addEventListener('click', async () => {
  const response = await chrome.runtime.sendMessage({ type: 'CLEAR_HISTORY' });
  if (response?.error) {
    statusEl.textContent = `清空失败：${response.error}`;
    return;
  }

  renderRecords([]);
});

loadRecords().catch((error) => {
  statusEl.textContent = `发生错误：${error.message}`;
});
