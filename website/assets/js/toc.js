/**
 * Copyright 2022 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
'use strict';

(function() {
const sidebar = document.getElementById('toc');
if (!sidebar) return;

// Data structures computed by examining DOM.
const headings = [];            // h1 and h2 elements found in the document.
const toc_entries = new Map();  // TOC <li> per h1 or h2 element.
const children = new Map();     // List of h2 child elements per h1.
const parent = new Map();       // Parent h1 element per h2.
let active = null;              // Current h1 or h2.
let active_h1 = null;           // Active h1 or parent of active h2.

init_toc();
bind();

function init_toc() {
  const sections = {
    'what-is-service-weaver': 'OVERVIEW',
    'components': 'FUNDAMENTALS',
    'single-process': 'DEPLOYERS',
    'serializable-types': 'REFERENCE',
  }

  // Process all headings.
  let hs = document.querySelectorAll('h1, h2');
  const toc_list = document.createElement('ul');
  let current_h1_id = '';
  let current_h1 = null;
  let current_h1_list = null;
  hs.forEach(h => {
    headings.push(h);

    // Assign id.
    let id = h.innerText;
    id = id.replaceAll(' ', '-');
    id = id.replaceAll(/[^-a-zA-Z0-9]/g, '');
    id = id.toLowerCase();
    if (h.tagName == 'H1') {
      current_h1_id = id;
    } else {
      id = current_h1_id + '-' + id;
    }
    h.id = id;

    // Generate TOC entry.
    const item = document.createElement('li');
    const link = link_to(h.innerText, '#' + id);
    item.appendChild(link);
    toc_entries.set(h, item);
    if (h.tagName == 'H1') {
      children.set(h, []);
      if (id in sections) {
        const section = document.createElement('li');
        section.innerText = sections[id];
        section.classList.add('toc-section');
        toc_list.appendChild(section);
      }
      toc_list.appendChild(item);
      current_h1 = h;
      current_h1_list = null;
    } else if (current_h1) {
      children.get(current_h1).push(h);
      parent.set(h, current_h1);
      if (!current_h1_list) {
        current_h1_list = document.createElement('ul');
        toc_entries.get(current_h1).appendChild(current_h1_list);
      }
      current_h1_list.appendChild(item);
    }
  });
  sidebar.appendChild(toc_list);
}

// get_active_h returns the currently active h1 or h2.
function get_active_h() {
  // We say a heading is "active" if it is within WIGGLE of the top of the
  // screen. This wiggle room makes the active section feel more natural, as you
  // typically aren't reading at the very tippy top of the screen.
  const WIGGLE = 200;
  for (let i = headings.length - 1; i >= 0; i--) {
    const h = headings[i];
    const top = h.getBoundingClientRect().y;
    if (top < WIGGLE) {
      return h;
    }
  }
  return null;
}

// link_to creates an <a> element with the provided text and href. For
// example, link_to('Click here', 'example.com') returns the following
// HTML element:
//
//     <a href="example.com">Click here</a>
function link_to(text, href) {
  let a = document.createElement('a');
  a.setAttribute('href', href);
  a.innerText = text;
  return a;
}

// update_sidebar updates the left sidebar with the currently active h1.
function update_sidebar(h) {
  if (!h || h == active) return;  // nothing to update
  const new_entry = toc_entries.get(h);
  if (!new_entry) return;  // Not a covered heading

  // Expand the new h1 section. Collapse previous h1 section.
  const old_h1_entry = toc_entries.get(active_h1);
  if (old_h1_entry) {
    old_h1_entry.classList.remove('toc-expand');
  }
  active_h1 = parent.get(h) || h;
  toc_entries.get(active_h1).classList.add('toc-expand');

  // Update the active element.
  const old = toc_entries.get(active)
  if (old) {
    const old_a = old.querySelector('a');
    if (old_a) {
      old_a.classList.remove('toc-active');
    }
  }
  const a = new_entry.querySelector('a');
  if (a) {
    a.classList.add('toc-active');
  }
  active = h;

  make_visible(sidebar, new_entry);
}

// make_visible adjusts sidebar scroll position if necessary to make item
// visible. We do not use scrollIntoView() since that gneerates scroll events
// that cause us to call refresh_active_section() again.
function make_visible(sidebar, item) {
  const pos = item.getBoundingClientRect();
  const spos = sidebar.getBoundingClientRect();
  const padding = 20;
  if (pos.top < spos.top) {
    sidebar.scrollTop -= padding + (spos.top - pos.top);
  } else if (pos.bottom > spos.bottom) {
    sidebar.scrollTop += padding + (pos.bottom - spos.bottom);
  }
}

// refresh_active_section refreshes the navigation with the active section.
function refresh_active_section() {
  let active = get_active_h();
  update_sidebar(active);
}

function bind() {
  document.addEventListener('scroll', refresh_active_section);
  window.addEventListener('load', refresh_active_section);
  // Prevent scrolling of TOC from invoking onScroll.
  sidebar.addEventListener('scroll', (e) => {e.stopPropagation()});
}
})();
