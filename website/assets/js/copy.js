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

// addCopyButtonsToPres adds a copy button to every code snippet inside a pre.
// Clicking this button copies the contents of the snippet to the clipboard.
//
// TODO(mwhittaker): Display a message letting the user know the contents were
// successfully copied.
function addCopyButtonsToPres() {
  for (let pre of document.querySelectorAll('pre')) {
    const code = pre.querySelector('code');
    if (!code) continue;

    // Create the button.
    let button = document.createElement('button');
    button.innerText = '📋';
    button.classList.add('pre-copy-button');
    button.addEventListener('click', function() {
      if (navigator.clipboard) {
        // Due to how the markdown renderer converts markdown to HTML, every
        // newline in innerText is duplicated. The replaceAll removes the
        // redundant newlines.
        navigator.clipboard.writeText(code.innerText.replaceAll('\n\n', '\n'));
      }
    });

    // Update the parent pre.
    pre.classList.add('pre-copy-button-parent');
    pre.appendChild(button);
  }
}

window.addEventListener('load', addCopyButtonsToPres);
