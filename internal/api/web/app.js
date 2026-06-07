/* ── State ──────────────────────────────────────── */
const state = {
  streaming: false,
  metricsVisible: false,
  metricsTimer: null,
};

/* ── Init ───────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  loadDocuments();
  addWelcome();
  loadMetrics();
  state.metricsTimer = setInterval(loadMetrics, 15_000);
});

/* ── Welcome message ─────────────────────────────── */
function addWelcome() {
  addBotMsg('Привет! Загрузите **CSV** или **XLSX** файл кнопкой 📎 слева, затем задавайте вопросы по данным. Поддерживаются сложные аналитические вопросы — система автоматически сгенерирует и выполнит SQL-запросы.');
}

/* ── Documents ──────────────────────────────────── */
async function loadDocuments() {
  try {
    const res = await fetch('/files');
    if (!res.ok) return;
    const data = await res.json();
    renderDocList(data.files || []);
  } catch (e) {
    console.error('loadDocuments:', e);
  }
}

function renderDocList(docs) {
  const el = id('docList');
  id('docCount').textContent = docs.length;

  if (!docs.length) {
    el.innerHTML = `
      <div class="empty-state">
        <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
          <polyline points="14 2 14 8 20 8"/>
          <line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/>
        </svg>
        <p>Нет загруженных<br>документов</p>
      </div>`;
    return;
  }

  el.innerHTML = docs.map(doc => {
    const date = new Date(doc.uploaded_at).toLocaleDateString('ru', { day: 'numeric', month: 'short', year: 'numeric' });
    const ext = doc.filename.split('.').pop().toUpperCase();
    return `
      <div class="doc-card" id="doc-${doc.id}">
        <div class="doc-card-head">
          <div class="doc-name">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="flex-shrink:0;margin-top:1px">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
            </svg>
            ${esc(doc.filename)}
          </div>
          <div class="doc-meta">
            <span>${esc(ext)}</span>
            <span>${date}</span>
            <span>${doc.tables_found} табл.</span>
            <span>${doc.chunks_count} чанков</span>
          </div>
        </div>
        <div class="doc-actions">
          <button class="doc-btn" onclick="showSchema('${doc.id}', '${esc(doc.filename)}')">
            <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/>
              <rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>
            </svg>
            Схема
          </button>
          <button class="doc-btn danger" onclick="confirmDelete('${doc.id}')">
            <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="3 6 5 6 21 6"/>
              <path d="M19 6l-1 14H6L5 6"/>
              <path d="M10 11v6"/><path d="M14 11v6"/>
            </svg>
            Удалить
          </button>
        </div>
      </div>`;
  }).join('');
}

function confirmDelete(docId) {
  const card = id(`doc-${docId}`);
  if (!card) return;
  card.querySelector('.doc-confirm')?.remove();
  const div = document.createElement('div');
  div.className = 'doc-confirm';
  div.innerHTML = `
    <span>Удалить файл?</span>
    <button class="btn-xs" onclick="this.closest('.doc-confirm').remove()">Отмена</button>
    <button class="btn-xs danger" onclick="deleteDoc('${docId}')">Удалить</button>`;
  card.appendChild(div);
}

async function deleteDoc(docId) {
  try {
    const res = await fetch(`/files/${docId}`, { method: 'DELETE' });
    if (res.ok || res.status === 204) {
      const card = id(`doc-${docId}`);
      if (card) {
        card.style.cssText = 'opacity:0;transition:opacity .2s;pointer-events:none';
        setTimeout(loadDocuments, 220);
      }
    } else {
      alert('Ошибка удаления файла');
    }
  } catch (e) {
    alert('Ошибка сети: ' + e.message);
  }
}

/* ── Schema modal ───────────────────────────────── */
async function showSchema(fileId, filename) {
  const overlay = id('schemaOverlay');
  const body    = id('schemaBody');
  id('schemaTitle').textContent = filename;
  body.innerHTML = '<div style="color:var(--text-muted);text-align:center;padding:24px">Загрузка…</div>';
  overlay.classList.add('open');

  try {
    const res = await fetch(`/files/${fileId}/schema`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    const tables = data.tables || [];

    if (!tables.length) {
      body.innerHTML = '<p style="color:var(--text-muted)">Схема не найдена</p>';
      return;
    }

    body.innerHTML = tables.map((t, i) => `
      <div class="schema-table-block">
        <div class="schema-table-title">
          Таблица ${i + 1}
          ${t.sheet_name ? `<span class="pill">Лист: ${esc(t.sheet_name)}</span>` : ''}
          <span class="pill">${t.row_count} строк</span>
          <span class="pill">${(t.columns || []).length} колонок</span>
        </div>
        ${t.summary ? `<div class="schema-summary">${esc(t.summary)}</div>` : ''}
        <table class="schema-col-table">
          <thead><tr><th>Колонка</th><th>Оригинал</th><th>Тип</th></tr></thead>
          <tbody>
            ${(t.columns || []).map(c => `
              <tr>
                <td><span class="col-name">${esc(c.name)}</span></td>
                <td><span class="col-orig">${esc(c.original_name)}</span></td>
                <td><span class="col-type">${esc(c.type)}</span></td>
              </tr>`).join('')}
          </tbody>
        </table>
      </div>`).join('');
  } catch (e) {
    body.innerHTML = `<p style="color:var(--error)">Ошибка загрузки схемы: ${esc(e.message)}</p>`;
  }
}

function closeSchema() {
  id('schemaOverlay').classList.remove('open');
}

/* ── File upload ─────────────────────────────────── */
const STAGES = [
  { label: 'Чтение файла, обнаружение таблиц (island detection)', delay: 0 },
  { label: 'Нормализация схемы и запись Parquet в S3',            delay: 700 },
  { label: 'Генерация HYDE-гипотез через LLM',                    delay: 1500 },
  { label: 'Вычисление эмбеддингов (dense + BM25 sparse)',        delay: 2600 },
  { label: 'Индексация чанков в Qdrant',                          delay: 4000 },
];

async function uploadFile(input) {
  const file = input.files[0];
  if (!file) return;
  input.value = '';

  const msgEl = addSystemMsg(renderUploadCard(file.name));
  const stageList = msgEl.querySelector('.stage-list');

  // Animate stages while waiting for the synchronous backend response
  setStage(stageList, 0, 'active');
  const timers = STAGES.slice(1).map((s, i) =>
    setTimeout(() => {
      setStage(stageList, i, 'done');
      setStage(stageList, i + 1, 'active');
    }, s.delay)
  );

  try {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch('/files', { method: 'POST', body: form });

    timers.forEach(clearTimeout);

    if (!res.ok) {
      const txt = await res.text();
      STAGES.forEach((_, i) => setStage(stageList, i, 'error'));
      addErrToCard(msgEl, `Ошибка ${res.status}: ${txt}`);
      return;
    }

    const doc = await res.json();
    STAGES.forEach((_, i) => setStage(stageList, i, 'done'));
    addSuccessToCard(msgEl, doc);
    await loadDocuments();
  } catch (e) {
    timers.forEach(clearTimeout);
    STAGES.forEach((_, i) => setStage(stageList, i, 'error'));
    addErrToCard(msgEl, e.message);
  }
}

function renderUploadCard(filename) {
  return `<div class="upload-card">
    <div class="upload-filename">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
        <polyline points="14 2 14 8 20 8"/>
      </svg>
      ${esc(filename)}
    </div>
    <ul class="stage-list">
      ${STAGES.map(s => `
        <li class="stage-item">
          <span class="stage-icon">·</span>
          <span>${esc(s.label)}</span>
        </li>`).join('')}
    </ul>
  </div>`;
}

function setStage(list, idx, st) {
  const items = list.querySelectorAll('.stage-item');
  if (!items[idx]) return;
  const item = items[idx];
  const icon = item.querySelector('.stage-icon');
  item.className = `stage-item ${st}`;
  switch (st) {
    case 'active': icon.innerHTML = '<span class="spinner"></span>'; break;
    case 'done':   icon.textContent = '✓'; break;
    case 'error':  icon.textContent = '✗'; break;
    default:       icon.textContent = '·';
  }
}

function addSuccessToCard(msgEl, doc) {
  const card = msgEl.querySelector('.upload-card');
  const div = document.createElement('div');
  div.className = 'upload-stats';
  div.innerHTML = `
    <span class="stat-pill">Таблиц: <strong>${doc.tables_found}</strong></span>
    <span class="stat-pill">Чанков: <strong>${doc.chunks_count}</strong></span>`;
  card.appendChild(div);
}

function addErrToCard(msgEl, msg) {
  const card = msgEl.querySelector('.upload-card');
  const div = document.createElement('div');
  div.style.cssText = 'color:var(--error);font-size:12px;margin-top:10px';
  div.textContent = msg;
  card.appendChild(div);
}

/* ── Query & streaming ───────────────────────────── */
async function sendQuery() {
  if (state.streaming) return;
  const input = id('queryInput');
  const query = input.value.trim();
  if (!query) return;

  input.value = '';
  autoResize(input);
  setStreamingState(true);

  addUserMsg(query);
  const { bubble, textEl } = addBotBubble();
  textEl.innerHTML = '<div class="thinking"><span></span><span></span><span></span></div>';

  let answerText = '';
  let firstToken = true;

  try {
    const res = await fetch('/query/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query, top_k: 10 }),
    });

    if (!res.ok) {
      textEl.innerHTML = `<span style="color:var(--error)">Ошибка запроса (HTTP ${res.status})</span>`;
      setStreamingState(false);
      return;
    }

    const reader  = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buf += decoder.decode(value, { stream: true });
      const lines = buf.split('\n');
      buf = lines.pop(); // keep partial line

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        let evt;
        try { evt = JSON.parse(line.slice(6)); } catch { continue; }

        if (evt.type === 'token') {
          if (firstToken) { textEl.innerHTML = ''; firstToken = false; }
          answerText += evt.content;
          // Use a text node + cursor span to avoid XSS and re-render cheaply
          textEl.textContent = answerText;
          const cur = document.createElement('span');
          cur.className = 'cursor';
          textEl.appendChild(cur);
          scrollToBottom();

        } else if (evt.type === 'result') {
          textEl.innerHTML = renderMd(evt.answer || answerText);
          renderSources(bubble, evt.sources, evt.sql_attempts);
          scrollToBottom();

        } else if (evt.type === 'error') {
          textEl.innerHTML = `<span style="color:var(--error)">${esc(evt.message)}</span>`;
        }
      }
    }
  } catch (e) {
    textEl.innerHTML = `<span style="color:var(--error)">${esc(e.message)}</span>`;
  }

  setStreamingState(false);
}

/* ── Sources ─────────────────────────────────────── */
function renderSources(bubble, sources, sqlAttempts) {
  const hasSql = sqlAttempts && sqlAttempts.length > 0;
  const hasSrc = sources && sources.length > 0;
  if (!hasSql && !hasSrc) return;

  const count = (sources?.length ?? 0) + (sqlAttempts?.length ?? 0);
  const section = document.createElement('div');
  section.className = 'sources-section';

  const toggle = document.createElement('button');
  toggle.className = 'sources-toggle';
  toggle.innerHTML = `
    <svg class="toggle-arrow" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <polyline points="9 18 15 12 9 6"/>
    </svg>
    Источники · ${count}`;

  const content = document.createElement('div');
  content.className = 'sources-content hidden';

  if (hasSql) {
    const lbl = document.createElement('div');
    lbl.className = 'section-label';
    lbl.textContent = 'SQL-запросы';
    content.appendChild(lbl);
    sqlAttempts.forEach(a => content.appendChild(renderSqlBlock(a)));
  }

  if (hasSrc) {
    if (hasSql) content.appendChild(makeDivider());
    const lbl = document.createElement('div');
    lbl.className = 'section-label';
    lbl.textContent = 'Чанки из векторной БД';
    content.appendChild(lbl);
    sources.forEach(s => content.appendChild(renderSourceCard(s)));
  }

  toggle.onclick = () => {
    const isOpen = !content.classList.contains('hidden');
    content.classList.toggle('hidden', isOpen);
    toggle.classList.toggle('open', !isOpen);
  };

  section.appendChild(toggle);
  section.appendChild(content);
  bubble.appendChild(section);
}

function renderSqlBlock(attempt) {
  const block = document.createElement('div');
  block.className = 'sql-block';

  // Table reference header
  if (attempt.source_file) {
    const ref = document.createElement('div');
    ref.className = 'sql-table-ref';
    const sheet  = attempt.sheet_name ? ` · ${esc(attempt.sheet_name)}` : '';
    const tIndex = ` · таблица ${attempt.table_index + 1}`;
    ref.innerHTML = `
      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="3" y="3" width="18" height="18" rx="2"/>
        <path d="M3 9h18M3 15h18M9 3v18"/>
      </svg>
      <span class="sql-ref-file">${esc(attempt.source_file)}</span>
      <span class="sql-ref-meta">${sheet}${tIndex}</span>`;
    block.appendChild(ref);
  }

  const code = document.createElement('div');
  code.className = 'sql-code';
  code.textContent = attempt.sql;
  block.appendChild(code);

  const rows = attempt.rows || [];
  if (rows.length > 0) {
    const keys = Object.keys(rows[0]);
    const wrap = document.createElement('div');
    wrap.className = 'sql-results';
    const table = document.createElement('table');
    table.className = 'data-table';
    table.innerHTML = `
      <thead><tr>${keys.map(k => `<th>${esc(k)}</th>`).join('')}</tr></thead>
      <tbody>${rows.map(row =>
        `<tr>${keys.map(k => `<td>${esc(String(row[k] ?? ''))}</td>`).join('')}</tr>`
      ).join('')}</tbody>`;
    wrap.appendChild(table);
    block.appendChild(wrap);
  } else {
    const empty = document.createElement('div');
    empty.className = 'sql-empty';
    empty.textContent = 'Нет результатов';
    block.appendChild(empty);
  }

  return block;
}

function renderSourceCard(src) {
  const card = document.createElement('div');
  card.className = 'source-card';
  const sheet = src.sheet_name ? ` · ${esc(src.sheet_name)}` : '';
  const text  = (src.text || '').slice(0, 240);
  card.innerHTML = `
    <div class="source-meta">
      <span class="source-file">${esc(src.source_file)}</span>
      <span>${sheet ? sheet + ' ·' : ''} табл.&nbsp;${src.table_index + 1}, стр.&nbsp;${src.row_index + 1}</span>
      <span class="score-badge">${(src.score * 100).toFixed(0)}%</span>
    </div>
    <div class="source-text">${esc(text)}${src.text.length > 240 ? '…' : ''}</div>`;
  return card;
}

function makeDivider() {
  const d = document.createElement('div');
  d.style.cssText = 'height:1px;background:var(--border-2);margin:4px 0 8px';
  return d;
}

/* ── Metrics ─────────────────────────────────────── */
async function loadMetrics() {
  try {
    const res = await fetch('/metrics');
    if (!res.ok) return;
    const m = await res.json();

    const ing = m.ingestion     || {};
    const qry = m.query         || {};
    const ddb = m.duckdb_cache  || {};

    const avgDur = qry.requests_total > 0
      ? Math.round(qry.total_duration_ms / qry.requests_total)
      : 0;

    const hits   = ddb.hits   ?? 0;
    const misses = ddb.misses ?? 0;

    setText('m-files',      ing.files_processed   ?? 0);
    setText('m-tables',     ing.tables_processed  ?? 0);
    setText('m-chunks',     ing.chunks_created    ?? 0);
    setText('m-queries',    qry.requests_total    ?? 0);
    setText('m-sql-path',   qry.sql_path          ?? 0);
    setText('m-text-path',  qry.text_path         ?? 0);
    setText('m-cache-hits', `${hits}/${hits + misses}`);
    setText('m-avg-dur',    avgDur);
  } catch (e) {
    // metrics endpoint may not be available — silent fail
  }
}

function toggleMetrics() {
  state.metricsVisible = !state.metricsVisible;
  id('metricsBar').classList.toggle('visible', state.metricsVisible);
  id('metricsBtn').classList.toggle('active',  state.metricsVisible);
  if (state.metricsVisible) loadMetrics();
}

/* ── Message builders ────────────────────────────── */
function addUserMsg(text) {
  const msgs = id('messages');
  const el = document.createElement('div');
  el.className = 'msg user';
  el.innerHTML = `
    <div class="msg-avatar">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="8" r="4"/>
        <path d="M5.5 21a8 8 0 0 1 13 0"/>
      </svg>
    </div>
    <div class="msg-body">
      <div class="msg-bubble"><p>${esc(text).replace(/\n/g, '<br>')}</p></div>
    </div>`;
  msgs.appendChild(el);
  scrollToBottom();
}

function addBotMsg(text) {
  const msgs = id('messages');
  const el = document.createElement('div');
  el.className = 'msg assistant';
  el.innerHTML = `
    <div class="msg-avatar">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 8V4H8"/>
        <rect width="16" height="12" x="4" y="8" rx="2"/>
        <path d="M2 14h2"/><path d="M20 14h2"/>
        <path d="M15 13v2"/><path d="M9 13v2"/>
      </svg>
    </div>
    <div class="msg-body">
      <div class="msg-bubble"><p>${renderMd(text)}</p></div>
    </div>`;
  msgs.appendChild(el);
  scrollToBottom();
  return el;
}

function addBotBubble() {
  const msgs = id('messages');
  const el   = document.createElement('div');
  el.className = 'msg assistant';

  const textEl = document.createElement('p');
  const bubble = document.createElement('div');
  bubble.className = 'msg-bubble';
  bubble.appendChild(textEl);

  const body = document.createElement('div');
  body.className = 'msg-body';
  body.appendChild(bubble);

  el.innerHTML = `
    <div class="msg-avatar">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 8V4H8"/>
        <rect width="16" height="12" x="4" y="8" rx="2"/>
        <path d="M2 14h2"/><path d="M20 14h2"/>
        <path d="M15 13v2"/><path d="M9 13v2"/>
      </svg>
    </div>`;
  el.appendChild(body);
  msgs.appendChild(el);
  scrollToBottom();
  return { bubble, textEl };
}

function addSystemMsg(html) {
  const msgs = id('messages');
  const el = document.createElement('div');
  el.className = 'msg system';
  el.innerHTML = `
    <div class="msg-avatar">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="16 3 21 3 21 8"/><polyline points="4 20 9 15"/><line x1="21" y1="3" x2="14" y2="10"/>
        <polyline points="8 21 3 21 3 16"/><line x1="3" y1="21" x2="10" y2="14"/>
      </svg>
    </div>
    <div class="msg-body">${html}</div>`;
  msgs.appendChild(el);
  scrollToBottom();
  return el;
}

/* ── Utilities ───────────────────────────────────── */
function setStreamingState(active) {
  state.streaming = active;
  id('sendBtn').disabled   = active;
  id('queryInput').disabled = active;
}

function handleKey(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendQuery();
  }
}

function autoResize(el) {
  el.style.height = 'auto';
  el.style.height = Math.min(el.scrollHeight, 150) + 'px';
}

function scrollToBottom() {
  const msgs = id('messages');
  msgs.scrollTop = msgs.scrollHeight;
}

function id(s)       { return document.getElementById(s); }
function setText(s, v) { const el = id(s); if (el) el.textContent = v; }

function esc(str) {
  return String(str ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/* ── Markdown renderer ───────────────────────────── */
function renderMd(raw) {
  const lines = (raw || '').split('\n');
  let out = '';
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block
    if (/^```/.test(line)) {
      const lang = line.slice(3).trim();
      const codeLines = [];
      i++;
      while (i < lines.length && !/^```/.test(lines[i])) {
        codeLines.push(lines[i]);
        i++;
      }
      i++; // skip closing ```
      out += `<pre><code${lang ? ` class="lang-${esc(lang)}"` : ''}>${esc(codeLines.join('\n'))}</code></pre>`;
      continue;
    }

    // Heading
    const hm = line.match(/^(#{1,6})\s+(.+)$/);
    if (hm) {
      const lvl = hm[1].length;
      out += `<h${lvl}>${inline(hm[2])}</h${lvl}>`;
      i++;
      continue;
    }

    // Horizontal rule
    if (/^[-*_]{3,}$/.test(line.trim()) && new Set(line.trim().split('')).size === 1) {
      out += '<hr>';
      i++;
      continue;
    }

    // Markdown table: header row followed by separator row (|---|---|)
    if (line.includes('|') && i + 1 < lines.length && isTableSep(lines[i + 1])) {
      const headers = splitTableRow(line);
      i += 2; // skip header + separator
      let html = '<table class="md-table"><thead><tr>';
      headers.forEach(h => { html += `<th>${inline(h)}</th>`; });
      html += '</tr></thead><tbody>';
      while (i < lines.length && lines[i].includes('|')) {
        const cells = splitTableRow(lines[i]);
        html += '<tr>';
        cells.forEach(c => { html += `<td>${inline(c)}</td>`; });
        html += '</tr>';
        i++;
      }
      html += '</tbody></table>';
      out += html;
      continue;
    }

    // Blockquote
    if (/^> ?/.test(line)) {
      const qLines = [];
      while (i < lines.length && /^> ?/.test(lines[i])) {
        qLines.push(lines[i].replace(/^> ?/, ''));
        i++;
      }
      out += `<blockquote>${renderMd(qLines.join('\n'))}</blockquote>`;
      continue;
    }

    // Unordered list
    if (/^[-*+] /.test(line)) {
      out += '<ul>';
      while (i < lines.length && /^[-*+] /.test(lines[i])) {
        out += `<li>${inline(lines[i].replace(/^[-*+] /, ''))}</li>`;
        i++;
      }
      out += '</ul>';
      continue;
    }

    // Ordered list
    if (/^\d+\. /.test(line)) {
      out += '<ol>';
      while (i < lines.length && /^\d+\. /.test(lines[i])) {
        out += `<li>${inline(lines[i].replace(/^\d+\. /, ''))}</li>`;
        i++;
      }
      out += '</ol>';
      continue;
    }

    // Empty line
    if (line.trim() === '') {
      i++;
      continue;
    }

    // Paragraph — collect consecutive non-special lines
    const paraLines = [];
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      !/^#{1,6} /.test(lines[i]) &&
      !/^```/.test(lines[i]) &&
      !/^> ?/.test(lines[i]) &&
      !/^[-*+] /.test(lines[i]) &&
      !/^\d+\. /.test(lines[i]) &&
      !/^[-*_]{3,}$/.test(lines[i].trim()) &&
      !(lines[i].includes('|') && i + 1 < lines.length && isTableSep(lines[i + 1]))
    ) {
      paraLines.push(lines[i]);
      i++;
    }
    if (paraLines.length) {
      out += `<p>${inline(paraLines.join('\n').replace(/\n/g, '<br>'))}</p>`;
    }
  }

  return out;
}

// Table helpers
function isTableSep(line) {
  return line.includes('|') && line.includes('-') && /^[\s|:\-]+$/.test(line);
}

function splitTableRow(line) {
  return line.replace(/^\||\|$/g, '').split('|').map(c => c.trim());
}

function inline(text) {
  let s = esc(text);

  // Protect inline code first
  const saved = [];
  s = s.replace(/`([^`\n]+?)`/g, (_, code) => {
    saved.push(`<code>${code}</code>`);
    return `\x00${saved.length - 1}\x00`;
  });

  // Bold
  s = s.replace(/\*\*([^*\n]+?)\*\*/g, '<strong>$1</strong>');
  s = s.replace(/__([^_\n]+?)__/g,     '<strong>$1</strong>');

  // Italic
  s = s.replace(/\*([^*\n]+?)\*/g, '<em>$1</em>');
  s = s.replace(/_([^_\n]+?)_/g,   '<em>$1</em>');

  // Restore inline code
  return s.replace(/\x00(\d+)\x00/g, (_, n) => saved[+n]);
}
