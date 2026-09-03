// Everything a directory page does in the browser. Nothing here talks to the
// server: the tables arrive complete and the pull command is already on the
// page.

// Drawn here rather than pulled from an icon set, because two rectangles are
// not worth a dependency or its licence. Constant markup, so innerHTML
// carries nothing a reader supplied.
const copyIcon = `<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor"
  stroke-width="1.5" stroke-linejoin="round" aria-hidden="true">
  <rect x="5.75" y="5.75" width="8.5" height="8.5" rx="1.5"/>
  <path d="M11.25 3.75v-1a1.5 1.5 0 0 0-1.5-1.5h-6a1.5 1.5 0 0 0-1.5 1.5v6a1.5 1.5 0 0 0 1.5 1.5h1"/>
</svg>`;

// Said rather than drawn: a tick means "something happened", the word means
// which thing. It is also the accessible name, so what is announced and what
// is on screen are the same string.
const copiedLabel = 'Copied!';

// The copy button is created here rather than shipped in the template, so a
// reader without JavaScript sees a plain command to select instead of a button
// that does nothing. Same reason it is skipped where the Clipboard API is
// absent, as it is on a plain-http origin.
if (navigator.clipboard !== undefined) {
  for (const target of document.querySelectorAll('[data-copyable]')) {
    copyable(target);
  }
}

function copyable(target) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'copy';
  button.innerHTML = copyIcon;
  // The icon is decoration, so the label is the button's only name. It says
  // what would be copied rather than just "Copy", because a page carries two
  // of these and "Copy" twice tells a screen-reader user nothing.
  button.setAttribute('aria-label', `Copy: ${target.textContent}`);
  button.setAttribute('title', 'Copy');
  // The label changes to report the result, so it has to be announced rather
  // than only redrawn.
  button.setAttribute('aria-live', 'polite');

  target.after(button);

  // The whole box copies, not just the button: it is a far bigger target for
  // a mouse. One handler serves both, because activating the button with the
  // keyboard fires a click that bubbles up to here.
  const region = target.closest('.command') ?? button;

  let restore = null;
  region.addEventListener('click', async () => {
    // Dragging across the command selects it, and a selection is a reader
    // saying they want this part rather than the whole line. The click that
    // ends that drag must not overrule them — this is also what keeps
    // dragging the box sideways to scroll it from copying.
    if (!window.getSelection().isCollapsed) return;

    try {
      await navigator.clipboard.writeText(target.textContent);
    } catch {
      // A refused clipboard is the browser's decision to explain, not a page
      // error to shout about. The command is still there to select.
      return;
    }
    // textContent replaces the icon outright, so there is no stale svg left
    // behind it.
    button.textContent = copiedLabel;
    button.setAttribute('aria-label', copiedLabel);
    button.setAttribute('title', copiedLabel);
    clearTimeout(restore);
    restore = setTimeout(() => {
      button.innerHTML = copyIcon;
      button.setAttribute('aria-label', `Copy: ${target.textContent}`);
      button.setAttribute('title', 'Copy');
    }, 1500);
  });
}

// A <details> stays open until its own summary is clicked again, which is not
// how anything shaped like a dropdown behaves: the next click elsewhere, or
// Escape, should dismiss it. Without JavaScript it still opens and closes on
// the summary, which is why the markup is a disclosure rather than a menu.
document.addEventListener('click', (event) => {
  for (const open of document.querySelectorAll('details.switcher[open]')) {
    if (!open.contains(event.target)) open.open = false;
  }
});

document.addEventListener('keydown', (event) => {
  if (event.key !== 'Escape') return;
  for (const open of document.querySelectorAll('details.switcher[open]')) {
    open.open = false;
    // Escape dismisses, so focus goes back to what was dismissed rather than
    // to the top of the document.
    open.querySelector('summary')?.focus();
  }
});

// A directory table arrives complete in the document, so sorting and filtering
// are local operations on rows that already exist.
//
// Found by marker rather than by id, because two pages share this file and
// neither should have to be named in it — and guarded, because a page may have
// a pull command and no table.
const table = document.querySelector('table[data-sortable]');
if (table !== null) {
  sortable(table);
}

function sortable(table) {
  const body = table.tBodies[0];

  // Read each row once; textContent is what the filter searches.
  const rows = Array.from(body.rows, (element) => ({
    element,
    haystack: element.textContent.toLowerCase(),
  }));

  // Filtering is the SBOM page's alone — a versions page lists a handful of
  // builds, and a search box over six rows earns nothing. So the controls are
  // optional, and the noun below is the only thing in this file that knows
  // which page brought them.
  const filter = document.getElementById('filter');
  const count = document.getElementById('count');
  if (filter !== null) {
    filter.addEventListener('input', () => {
      const needle = filter.value.trim().toLowerCase();
      let shown = 0;
      for (const row of rows) {
        const matches = needle === '' || row.haystack.includes(needle);
        row.element.hidden = !matches;
        if (matches) shown += 1;
      }
      count.textContent = needle === ''
        ? `${rows.length} components`
        : `${shown} of ${rows.length} components`;
    });
  }

  table.tHead.addEventListener('click', (event) => {
    const header = event.target.closest('th[data-column]');
    if (header === null) return;

    const descending = header.getAttribute('aria-sort') === 'ascending';
    for (const other of table.tHead.rows[0].cells) {
      other.setAttribute('aria-sort', 'none');
    }
    header.setAttribute('aria-sort', descending ? 'descending' : 'ascending');

    // A cell may carry its own sort key, for the columns whose rendered text
    // does not sort the way the value does: dpkg version order, which the
    // server computed rather than reimplement here, and a size, where "9 MB"
    // would otherwise land above "23.9 MB". Everything else sorts as text —
    // which is why the build horizon is rendered `YYYY-MM-DD`, an ISO date
    // being one that already sorts correctly as a string.
    const index = header.cellIndex;
    const key = (row) => {
      const cell = row.element.cells[index];
      return cell.dataset.sortKey === undefined ? cell.textContent : Number(cell.dataset.sortKey);
    };

    // Array.prototype.sort is stable, so ties keep the order of the previous
    // sort — click Version then License and you get license-major,
    // version-minor.
    const direction = descending ? -1 : 1;
    rows.sort((a, b) => {
      const [left, right] = [key(a), key(b)];
      if (left < right) return -direction;
      if (left > right) return direction;
      return 0;
    });

    // append() moves nodes it already owns, so this reorders rather than
    // rebuilds.
    body.append(...rows.map((row) => row.element));
  });
}
