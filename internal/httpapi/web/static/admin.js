(() => {
  'use strict';
  const state = { token: '', poll: null, timer: null, qrURL: null };
  const publicBaseURL = document.body.dataset.publicBaseUrl || location.origin;
  const byId = (id) => document.getElementById(id);
  const status = byId('admin-status');
  const summary = byId('poll-summary');
  const badge = byId('poll-badge');
  const actions = byId('poll-actions');
  const share = byId('share-panel');
  const results = byId('results');
  const pollList = byId('poll-list');

  function message(text, kind = '') {
    status.textContent = text;
    status.className = `status ${kind}`.trim();
  }

  function setHidden(element, hidden) {
    element.hidden = hidden;
    element.classList.toggle('is-hidden', hidden);
  }

  async function api(path, options = {}) {
    if (!state.token) throw new Error('Сначала введите admin token.');
    const headers = new Headers(options.headers || {});
    headers.set('Authorization', `Bearer ${state.token}`);
    const response = await fetch(path, { ...options, headers });
    if (!response.ok) {
      let text = `Ошибка ${response.status}`;
      try {
        const body = await response.json();
        text = body.error?.message || text;
      } catch {}
      throw new Error(text);
    }
    return response;
  }

  function localValue(date) {
    const offset = date.getTimezoneOffset() * 60000;
    return new Date(date.getTime() - offset).toISOString().slice(0, 16);
  }

  const now = new Date();
  byId('starts-at').value = localValue(now);
  byId('ends-at').value = localValue(new Date(now.getTime() + 5 * 60 * 1000));

  function syncType() {
    const single = byId('poll-type').value === 'single_choice';
    byId('max-choices').value = single ? '1' : Math.max(2, Number(byId('max-choices').value));
    byId('max-choices').disabled = single;
  }

  byId('poll-type').addEventListener('change', syncType);
  syncType();

  byId('token-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    state.token = byId('admin-token').value;
    byId('admin-token').value = '';
    message('Токен принят. Загружаем сохранённые опросы…');
    await loadPolls();
  });

  function clearQR() {
    if (state.qrURL) URL.revokeObjectURL(state.qrURL);
    state.qrURL = null;
    byId('qr-preview').removeAttribute('src');
    byId('qr-download').removeAttribute('href');
  }

  function renderPoll(poll) {
    state.poll = poll;
    badge.textContent = poll.status;
    summary.className = 'poll-meta';
    summary.replaceChildren();
    results.replaceChildren();

    const title = document.createElement('strong');
    title.textContent = poll.question;
    const id = document.createElement('span');
    id.className = 'poll-id';
    id.textContent = poll.id;
    const timing = document.createElement('span');
    timing.className = 'muted';
    timing.textContent = `${new Date(poll.starts_at).toLocaleString()} — ${new Date(poll.ends_at).toLocaleString()}`;
    summary.append(title, id, timing);

    setHidden(actions, false);
    byId('publish-button').disabled = poll.status !== 'draft';
    byId('refresh-button').disabled = poll.status === 'draft';
    byId('close-button').disabled = poll.status !== 'active';

    const publicURL = `${publicBaseURL}/p/${encodeURIComponent(poll.slug)}`;
    const link = byId('public-link');
    link.href = publicURL;
    link.textContent = publicURL;
    setHidden(share, poll.status === 'draft');
    clearQR();
    if (poll.status !== 'draft') loadQR(poll.id, poll.slug);
    if (poll.status === 'active') startPolling();
    else {
      stopPolling();
      if (poll.status === 'closed') loadResults();
    }
  }

  function renderPollList(polls) {
    pollList.replaceChildren();
    if (polls.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'Сохранённых опросов пока нет.';
      pollList.append(empty);
      return;
    }
    polls.forEach((poll) => {
      const row = document.createElement('div');
      row.className = 'poll-list-item';
      const main = document.createElement('div');
      main.className = 'poll-list-main';
      const title = document.createElement('span');
      title.className = 'poll-list-title';
      title.textContent = poll.question;
      const meta = document.createElement('span');
      meta.className = 'poll-list-meta';
      meta.textContent = `${poll.status} · ${new Date(poll.created_at).toLocaleString()} · ${poll.slug}`;
      main.append(title, meta);
      const open = document.createElement('button');
      open.type = 'button';
      open.textContent = 'Открыть';
      open.addEventListener('click', () => {
        renderPoll(poll);
        message(`Открыт опрос «${poll.question}».`, 'success');
        document.querySelector('.current-card').scrollIntoView({ behavior: 'smooth', block: 'start' });
      });
      row.append(main, open);
      pollList.append(row);
    });
  }

  async function loadPolls(quiet = false) {
    try {
      const response = await api('/api/v1/admin/polls?limit=100');
      const data = await response.json();
      renderPollList(data.polls);
      if (!quiet) message(`Загружено опросов: ${data.polls.length}.`, 'success');
    } catch (error) {
      pollList.replaceChildren();
      const empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'Не удалось загрузить сохранённые опросы.';
      pollList.append(empty);
      if (!quiet) message(error.message, 'error');
    }
  }

  byId('reload-polls').addEventListener('click', () => loadPolls());

  byId('create-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const optionTexts = byId('options-input').value.split('\n').map((text) => text.trim()).filter(Boolean);
    const body = {
      question: byId('question-input').value,
      type: byId('poll-type').value,
      max_choices: Number(byId('max-choices').value),
      starts_at: new Date(byId('starts-at').value).toISOString(),
      ends_at: new Date(byId('ends-at').value).toISOString(),
      options: optionTexts.map((text) => ({ text })),
    };
    try {
      message('Создаём опрос…');
      const response = await api('/api/v1/admin/polls', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      renderPoll(await response.json());
      await loadPolls(true);
      message('Черновик создан.', 'success');
    } catch (error) {
      message(error.message, 'error');
    }
  });

  async function action(name) {
    if (!state.poll) return;
    try {
      message(name === 'publish' ? 'Публикуем…' : 'Закрываем…');
      const response = await api(`/api/v1/admin/polls/${state.poll.id}/${name}`, { method: 'POST' });
      const data = await response.json();
      if (name === 'publish') renderPoll(data);
      else {
        state.poll.status = 'closed';
        renderPoll(state.poll);
        renderResults(data);
      }
      await loadPolls(true);
      message(name === 'publish' ? 'Опрос опубликован.' : 'Опрос закрыт, результат сохранён.', 'success');
    } catch (error) {
      message(error.message, 'error');
    }
  }

  byId('publish-button').addEventListener('click', () => action('publish'));
  byId('close-button').addEventListener('click', () => action('close'));
  byId('refresh-button').addEventListener('click', loadResults);

  function renderResults(data) {
    results.replaceChildren();
    const head = document.createElement('p');
    head.className = 'muted';
    head.textContent = `Участников: ${data.participants_count} · источник: ${data.source}`;
    results.append(head);
    data.options.forEach((option) => {
      const row = document.createElement('div');
      const top = document.createElement('div');
      top.className = 'result-head';
      const name = document.createElement('span');
      name.textContent = option.text;
      const value = document.createElement('strong');
      value.textContent = `${option.count} · ${option.participant_percent.toFixed(1)}%`;
      top.append(name, value);
      const progress = document.createElement('progress');
      progress.className = 'result-progress';
      progress.max = 100;
      progress.value = Math.min(100, option.participant_percent);
      row.append(top, progress);
      results.append(row);
    });
  }

  async function loadResults() {
    if (!state.poll || state.poll.status === 'draft') return;
    const pollID = state.poll.id;
    try {
      const response = await api(`/api/v1/admin/polls/${pollID}/results`);
      const data = await response.json();
      if (state.poll?.id === pollID) renderResults(data);
    } catch (error) {
      if (state.poll?.id === pollID) message(error.message, 'error');
    }
  }

  async function loadQR(pollID, slug) {
    try {
      const response = await api(`/api/v1/admin/polls/${pollID}/qr.svg`);
      const blob = await response.blob();
      if (state.poll?.id !== pollID) return;
      if (state.qrURL) URL.revokeObjectURL(state.qrURL);
      state.qrURL = URL.createObjectURL(blob);
      byId('qr-preview').src = state.qrURL;
      byId('qr-download').href = state.qrURL;
      byId('qr-download').download = `${slug}.svg`;
    } catch (error) {
      if (state.poll?.id === pollID) message(error.message, 'error');
    }
  }

  function startPolling() {
    stopPolling();
    loadResults();
    state.timer = setInterval(loadResults, 1000);
  }

  function stopPolling() {
    if (state.timer) clearInterval(state.timer);
    state.timer = null;
  }

  window.addEventListener('beforeunload', () => {
    stopPolling();
    if (state.qrURL) URL.revokeObjectURL(state.qrURL);
  });
})();
