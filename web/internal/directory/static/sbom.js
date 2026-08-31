// The whole SBOM is already in the document, so sorting and filtering are
// local operations on rows that exist. Nothing here talks to the server.
const table = document.getElementById('sbom');
const body = table.tBodies[0];
const count = document.getElementById('count');
const filter = document.getElementById('filter');

// Read each row once. textContent is the searchable text and dataset carries
// the version ordering the server worked out.
const rows = Array.from(body.rows, (element) => ({
  element,
  haystack: element.textContent.toLowerCase(),
  versionRank: Number(element.dataset.versionRank),
}));

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

table.tHead.addEventListener('click', (event) => {
  const header = event.target.closest('th[data-column]');
  if (header === null) return;

  const descending = header.getAttribute('aria-sort') === 'ascending';
  for (const other of table.tHead.rows[0].cells) {
    other.setAttribute('aria-sort', 'none');
  }
  header.setAttribute('aria-sort', descending ? 'descending' : 'ascending');

  // Versions sort on the rank the server computed; every other column is text.
  const index = header.cellIndex;
  const key = header.dataset.column === 'version'
    ? (row) => row.versionRank
    : (row) => row.element.cells[index].textContent;

  // Array.prototype.sort is stable, so ties keep the order of the previous
  // sort — click Version then License and you get license-major, version-minor.
  const direction = descending ? -1 : 1;
  rows.sort((a, b) => {
    const [left, right] = [key(a), key(b)];
    if (left < right) return -direction;
    if (left > right) return direction;
    return 0;
  });

  // append() moves nodes it already owns, so this reorders rather than rebuilds.
  body.append(...rows.map((row) => row.element));
});
