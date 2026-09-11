(() => {
  'use strict';
  const status = document.querySelector('#status');
  const form = document.querySelector('#vote-form');
  const question = document.querySelector('#question');
  const endsAt = document.querySelector('#ends-at');
  const options = document.querySelector('#options');
  const help = document.querySelector('#choice-help');
  const submit = document.querySelector('#submit-vote');
  const slug = decodeURIComponent(location.pathname.replace(/^\/p\//, ''));
  let poll;

  function message(text, kind = '') { status.textContent = text; status.className = `status ${kind}`.trim(); }
  function setHidden(element, hidden) { element.hidden = hidden; element.classList.toggle('is-hidden', hidden); }
  async function errorMessage(response) { try { const body = await response.json(); return body.error?.message || `Ошибка ${response.status}`; } catch { return `Ошибка ${response.status}`; } }
  function selected() { return [...options.querySelectorAll('input:checked')]; }
  function updateLimit() {
    if (!poll || poll.type !== 'multiple_choice') return;
    const chosen = selected().length;
    help.textContent = `Выбрано ${chosen} из ${poll.max_choices}`;
    options.querySelectorAll('input:not(:checked)').forEach((input) => { input.disabled = chosen >= poll.max_choices; });
  }
  function render(data) {
    poll = data; question.textContent = data.question;
    endsAt.textContent = `Голосование до ${new Date(data.ends_at).toLocaleString()}`;
    help.textContent = data.type === 'single_choice' ? 'Выберите один вариант' : `Можно выбрать до ${data.max_choices} вариантов`;
    data.options.forEach((option) => {
      const label = document.createElement('label'); label.className = 'option';
      const input = document.createElement('input'); input.type = data.type === 'single_choice' ? 'radio' : 'checkbox'; input.name = 'answer'; input.value = option.id; input.addEventListener('change', updateLimit);
      const text = document.createElement('span'); text.textContent = option.text;
      label.append(input, text); options.append(label);
    });
    status.textContent = ''; setHidden(form, false);
  }
  async function load() {
    if (!slug) { message('Некорректная ссылка на опрос.', 'error'); return; }
    try { const response = await fetch(`/api/v1/polls/${encodeURIComponent(slug)}`, { credentials: 'same-origin' }); if (!response.ok) throw new Error(await errorMessage(response)); render(await response.json()); }
    catch (error) { message(error.message || 'Опрос временно недоступен.', 'error'); }
  }
  form.addEventListener('submit', async (event) => {
    event.preventDefault(); const optionIds = selected().map((input) => input.value);
    if (optionIds.length === 0) { message('Выберите хотя бы один вариант.', 'error'); return; }
    submit.disabled = true; message('Отправляем голос…');
    try {
      const response = await fetch(`/api/v1/polls/${poll.id}/votes`, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ option_ids: optionIds }) });
      if (!response.ok) throw new Error(await errorMessage(response)); const result = await response.json();
      options.querySelectorAll('input').forEach((input) => { input.disabled = true; });
      message(result.status === 'already_recorded' ? 'Ваш голос уже был учтён.' : 'Спасибо! Ваш голос принят.', 'success');
    } catch (error) { submit.disabled = false; message(error.message || 'Не удалось отправить голос.', 'error'); updateLimit(); }
  });
  load();
})();
