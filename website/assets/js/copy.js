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
            let text = code.innerText.replaceAll('\n\n', '\n');

            // If the text contains commands, only copy the commands. Otherwise,
            // copy the entire text.
            let commands = getCommands(text);
            if (commands.length > 0) {
                text = commands;
            }
            navigator.clipboard.writeText(text);
        }
    });

    // Update the parent pre.
    pre.classList.add('pre-copy-button-parent');
    pre.appendChild(button);
  }
}

// getCommands splits the provided text into lines and returns the subset
// of lines that represent commands, i.e., the lines that start with "$ ".
function getCommands(text) {
    let lines = text.split("\n");
    let commands = [];
    for (let line of lines) {
        if (line.startsWith("$ ")) {
            commands.push(line.substring(2));
        }
    }
    return commands.join("\n");
}

window.addEventListener('load', addCopyButtonsToPres);
