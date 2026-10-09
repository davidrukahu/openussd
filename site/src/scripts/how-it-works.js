/**
 * How it works: the pills highlight one stage of the static flow diagram and
 * show its caption. Without JS every caption stays visible.
 */
export function init() {
  const panel = document.getElementById('hiw-panel');
  if (!panel) return;
  const pills = Array.from(panel.querySelectorAll('.hiw__pill'));
  const captions = Array.from(panel.querySelectorAll('.flow__caption'));
  panel.classList.add('is-interactive');

  const setStep = (step) => {
    panel.dataset.step = String(step);
    pills.forEach((p, i) => {
      p.classList.toggle('is-active', i === step);
      p.setAttribute('aria-pressed', i === step ? 'true' : 'false');
    });
    captions.forEach((c, i) => c.classList.toggle('is-active', i === step));
  };

  pills.forEach((p, i) => p.addEventListener('click', () => setStep(i)));
  setStep(0);
}
