// What the Go tests cannot reach: the behaviours themselves, called.
//
// page_test.go renders the templates and checks the markup they emit — the
// data-copyable marker, the sortable columns, the switcher's links. None of
// that executes a line of directory.mjs, so a reference error, a sort that
// compares strings where it meant numbers, or a copy button that reports
// success after the clipboard refused all pass a green `bazel test` and break
// only in a reader's browser. Every case below stands for a defect that
// actually got written here.
//
// directory.mjs exports behaviour and runs nothing on import, so these are
// ordinary function calls on fake elements. The DOM is stubbed rather than
// emulated: enough for the code under test, and no more. A stub that is wrong
// in the same way the code is wrong proves nothing, so the parts that bite are
// modelled properly — textContent and innerHTML replacing one another, a
// selection that is not collapsed — rather than storing whatever they are
// handed.

import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { copyable, dismissSwitchers, sortable } from './static/directory.mjs';

// textContent and innerHTML are accessors because the DOM makes them
// alternative views of the same content: setting one clears the other, which
// is exactly what the copy button relies on to replace its icon with a word.
function node(extra = {}) {
  return {
    attrs: {},
    handlers: {},
    _html: '',
    _text: '',
    get innerHTML() {
      return this._html;
    },
    set innerHTML(value) {
      this._html = value;
      this._text = '';
    },
    get textContent() {
      return this._text;
    },
    set textContent(value) {
      this._text = value;
      this._html = '';
    },
    setAttribute(name, value) {
      this.attrs[name] = value;
    },
    getAttribute(name) {
      return this.attrs[name];
    },
    addEventListener(event, fn) {
      this.handlers[event] = fn;
    },
    ...extra,
  };
}

// A document, as much of one as anything here reaches through ownerDocument.
function fakeDocument({ selectionCollapsed = true, elements = {}, matching = {} } = {}) {
  const listeners = {};
  return {
    created: [],
    getSelection: () => ({ isCollapsed: selectionCollapsed }),
    getElementById: (id) => elements[id] ?? null,
    querySelectorAll: (selector) => matching[selector] ?? [],
    addEventListener(event, fn) {
      (listeners[event] ??= []).push(fn);
    },
    fire(event, argument) {
      (listeners[event] ?? []).forEach((fn) => fn(argument));
    },
    createElement() {
      const made = node();
      this.created.push(made);
      return made;
    },
  };
}

// A command line: the copyable code, and the .command box it sits in, which is
// what actually carries the click handler.
function commandLine({ selectionCollapsed = true, text = 'docker pull distroless.io/java' } = {}) {
  const document = fakeDocument({ selectionCollapsed });
  const box = node();
  const target = node({
    ownerDocument: document,
    inserted: null,
    after(element) {
      this.inserted = element;
    },
    closest: (selector) => (selector === '.command' ? box : null),
  });
  target.textContent = text;
  return { document, box, target };
}

// A wired-up copy button, and the clipboard it was given.
function copyFixture({ writeText = async () => {}, selectionCollapsed = true } = {}) {
  const { document, box, target } = commandLine({ selectionCollapsed });
  copyable(target, { writeText });
  return { document, box, target, button: document.created[0] };
}

// A table of rows whose cells may carry their own sort key.
function fakeTable(headers, rows, { elements = {} } = {}) {
  const document = fakeDocument({ elements });
  const body = {
    rows: rows.map((row) => {
      const cells = row.map(([text, sortKey]) => ({
        textContent: text,
        dataset: sortKey === undefined ? {} : { sortKey: String(sortKey) },
      }));
      const element = { cells, hidden: false };
      Object.defineProperty(element, 'textContent', {
        get: () => cells.map((cell) => cell.textContent).join(' '),
      });
      return element;
    }),
    append(...ordered) {
      this.rows = ordered;
    },
  };
  const head = headers.map((column, index) => ({
    dataset: { column },
    cellIndex: index,
    sort: 'none',
    getAttribute() {
      return this.sort;
    },
    setAttribute(_, value) {
      this.sort = value;
    },
  }));
  let click = null;
  return {
    head,
    column: (index) => body.rows.map((row) => row.cells[index].textContent).join(' '),
    hidden: () => body.rows.map((row) => row.hidden),
    click: (index) => click({ target: { closest: () => head[index] } }),
    table: {
      ownerDocument: document,
      tBodies: [body],
      tHead: {
        rows: [{ cells: head }],
        addEventListener: (_, fn) => {
          click = fn;
        },
      },
    },
  };
}

const isCopyIcon = (html) => html.includes('<rect');

describe('copy button', () => {
  it('follows the command it copies', () => {
    const { target, button } = copyFixture();

    assert.equal(target.inserted, button);
    assert.ok(isCopyIcon(button.innerHTML), button.innerHTML);
  });

  it('names what it would copy, and hides its icon from screen readers', () => {
    // The icon is decoration, so the label is the button's only name — and a
    // page carries two of these, where "Copy" twice would say nothing.
    const { button } = copyFixture();

    assert.equal(button.attrs['aria-label'], 'Copy: docker pull distroless.io/java');
    assert.equal(button.attrs['aria-live'], 'polite');
    assert.ok(button.innerHTML.includes('aria-hidden="true"'));
  });

  it('makes the whole box the click target, not just itself', () => {
    // One handler serves both, because activating the button by keyboard
    // bubbles a click up to the box.
    const { box, button } = copyFixture();

    assert.equal(typeof box.handlers.click, 'function');
    assert.equal(button.handlers.click, undefined);
  });

  it('copies the command verbatim and says so in words', async () => {
    let written = null;
    const { box, button } = copyFixture({ writeText: async (text) => { written = text; } });

    await box.handlers.click();

    assert.equal(written, 'docker pull distroless.io/java');
    assert.equal(button.textContent, 'Copied!');
    // textContent replaces innerHTML, so the icon is gone rather than lurking
    // behind the word.
    assert.equal(button.innerHTML, '');
    assert.equal(button.attrs['aria-label'], 'Copied!');
  });

  it('leaves a selection alone rather than overruling it', async () => {
    // Dragging across the command selects part of it, and the click that ends
    // that drag must not copy the whole line instead. This is also what stops
    // dragging the box sideways to scroll it from copying.
    let written = null;
    const { box, button } = copyFixture({
      writeText: async (text) => { written = text; },
      selectionCollapsed: false,
    });

    await box.handlers.click();

    assert.equal(written, null);
    assert.ok(isCopyIcon(button.innerHTML));
    assert.notEqual(button.textContent, 'Copied!');
  });

  it('claims nothing when the clipboard refuses', async () => {
    // Reporting a copy that never happened is worse than not copying.
    const { box, button } = copyFixture({
      writeText: async () => { throw new Error('denied'); },
    });

    await box.handlers.click();

    assert.ok(isCopyIcon(button.innerHTML));
    assert.notEqual(button.textContent, 'Copied!');
  });
});

describe('switcher dismissal', () => {
  function openSwitcher() {
    const state = { focused: false };
    const summary = { focus() { state.focused = true; } };
    const inside = {};
    const switcher = {
      open: true,
      contains: (element) => element === inside || element === summary,
      querySelector: () => summary,
    };
    // querySelectorAll re-runs per event, so a closed switcher drops out the
    // way the real selector would.
    const document = fakeDocument();
    document.querySelectorAll = (selector) =>
      (selector === 'details.switcher[open]' && switcher.open ? [switcher] : []);
    dismissSwitchers(document);
    return { document, switcher, inside, state };
  }

  it('stays open for a click inside it', () => {
    const { document, switcher, inside } = openSwitcher();
    document.fire('click', { target: inside });
    assert.equal(switcher.open, true);
  });

  it('closes on a click elsewhere', () => {
    const { document, switcher } = openSwitcher();
    document.fire('click', { target: {} });
    assert.equal(switcher.open, false);
  });

  it('ignores keys that are not Escape', () => {
    const { document, switcher } = openSwitcher();
    document.fire('keydown', { key: 'a' });
    assert.equal(switcher.open, true);
  });

  it('closes on Escape and gives focus back to the summary', () => {
    // Otherwise dismissing by keyboard drops the reader at the top of the
    // document.
    const { document, switcher, state } = openSwitcher();

    document.fire('keydown', { key: 'Escape' });

    assert.equal(switcher.open, false);
    assert.equal(state.focused, true);
  });
});

describe('sorting', () => {
  it('sorts a size column as a number', () => {
    // Rendered "9.4 MB" and "180.2 MB". Sorted as text, 180.2 lands above 9.4
    // — so the server hands the browser the byte count to sort on instead.
    const t = fakeTable(['tags', 'created', 'size'], [
      [['latest'], ['2026-09-02'], ['23.9 MB', 23867899]],
      [['slim'], ['2026-09-02'], ['9.4 MB', 9400000]],
      [['big'], ['2026-05-02'], ['180.2 MB', 180200000]],
    ]);
    sortable(t.table);

    t.click(2);
    assert.equal(t.column(2), '9.4 MB 23.9 MB 180.2 MB');

    t.click(2);
    assert.equal(t.column(2), '180.2 MB 23.9 MB 9.4 MB');
  });

  it('marks only the column it sorted', () => {
    const t = fakeTable(['tags', 'size'], [[['a'], ['1 B', 1]], [['b'], ['2 B', 2]]]);
    sortable(t.table);

    t.click(1);

    assert.equal(t.head[1].sort, 'ascending');
    assert.equal(t.head[0].sort, 'none');
  });

  it('sorts versions on the rank the server worked out', () => {
    // dpkg order: 1.9 is below 1.10, and no string comparison gets there. It
    // rides the same per-cell key the size column uses, so the script needs no
    // branch naming either page.
    const t = fakeTable(['name', 'version'], [
      [['a'], ['1.10', 1]],
      [['b'], ['1.9', 0]],
      [['c'], ['2.0', 2]],
    ]);
    sortable(t.table);

    t.click(1);

    assert.equal(t.column(1), '1.9 1.10 2.0');
  });

  it('sorts a column without a key as text', () => {
    // Which is why the build horizon is rendered YYYY-MM-DD: an ISO date is
    // one that already sorts correctly as a string.
    const t = fakeTable(['created'], [[['2026-09-02']], [['unknown']], [['2026-05-02']]]);
    sortable(t.table);

    t.click(0);

    assert.equal(t.column(0), '2026-05-02 2026-09-02 unknown');
  });
});

describe('filter', () => {
  function filterFixture({ noun = 'components' } = {}) {
    const filter = node({ value: '' });
    const count = node({ dataset: { noun } });
    const t = fakeTable(['name'], [[['libc6']], [['openssl']], [['zlib1g']]],
      { elements: { filter, count } });
    sortable(t.table);
    return { t, filter, count };
  }

  it('counts in the noun the page gave it', () => {
    // Two pages share this file and count different things; the page says
    // which, so the script never has to be told which page it is on.
    const { filter, count } = filterFixture({ noun: 'findings' });

    filter.value = 'ssl';
    filter.handlers.input();

    assert.equal(count.textContent, '1 of 3 findings');
  });

  it('hides what does not match, and counts what is left', () => {
    const { t, filter, count } = filterFixture();

    filter.value = 'ssl';
    filter.handlers.input();

    assert.deepEqual(t.hidden(), [true, false, true]);
    assert.equal(count.textContent, '1 of 3 components');
  });

  it('restores every row when cleared', () => {
    const { t, filter, count } = filterFixture();

    filter.value = 'ssl';
    filter.handlers.input();
    filter.value = '';
    filter.handlers.input();

    assert.deepEqual(t.hidden(), [false, false, false]);
    assert.equal(count.textContent, '3 components');
  });

  it('is optional — the versions page has none', () => {
    // The script is shared, so a table without controls must still sort.
    const t = fakeTable(['name'], [[['b']], [['a']]]);

    assert.doesNotThrow(() => sortable(t.table));

    t.click(0);
    assert.equal(t.column(0), 'a b');
  });
});
