// What a directory page runs on load: find the elements, hand them to the
// behaviours in directory.mjs. All the branching about which page this is
// lives here, so nothing in directory.mjs has to ask.

import { copyable, dismissSwitchers, sortable } from './directory.mjs';

// No Clipboard API — a plain-http origin — means no copy button at all,
// rather than one that cannot copy.
if (navigator.clipboard !== undefined) {
  for (const target of document.querySelectorAll('[data-copyable]')) {
    copyable(target, navigator.clipboard);
  }
}

dismissSwitchers(document);

// Found by marker rather than by id, because two pages share this file and
// neither should have to be named in it — and guarded, because a page may
// have a pull command and no table.
const table = document.querySelector('table[data-sortable]');
if (table !== null) {
  sortable(table);
}
