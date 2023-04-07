/**
 * Copyright 2023 Google LLC
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

// TODO(mwhittaker): Debounce.

'use strict';

async function wrap(s, n) {
  const response = await fetch(`/wrap?s=${encodeURIComponent(s)}&n=${n}`);
  return await response.text();
}

function main() {
  const linewidth = document.getElementById('linewidth');
  const unwrapped = document.getElementById('unwrapped');
  const wrapped = document.getElementById('wrapped');
  const copy = document.getElementById('copy');
  const copied = document.getElementById('copied');

  linewidth.addEventListener('change', () => {
    wrapped.style.width = `${linewidth.value}ch`;
    wrap(unwrapped.value, linewidth.value).then((s) => {
      wrapped.innerHTML = s;
    });
  });

  unwrapped.addEventListener('input', () => {
    console.log(linewidth.value);
    wrap(unwrapped.value, linewidth.value).then((s) => {
      wrapped.innerHTML = s;
    });
  });

  let timer;
  copy.addEventListener('click', function() {
    if (navigator.clipboard) {
      navigator.clipboard.writeText(wrapped.innerHTML);
      copied.hidden = false;
      clearTimeout(timer);
      timer = setTimeout(() => copied.hidden = true, 1000);
    }
  });
}

document.addEventListener('DOMContentLoaded', main);
