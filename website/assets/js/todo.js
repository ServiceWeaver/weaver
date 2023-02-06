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

// show_todos unhides all TODOs if the ?dev or ?todo or ?internal queries are
// provided in the URL.
function show_todos() {
  // https://stackoverflow.com/a/901144
  const params = new Proxy(new URLSearchParams(window.location.search), {
    get: (searchParams, prop) => searchParams.get(prop),
  });
  if (params.dev != null || params.todo != null || params.internal != null) {
    for (let todo of document.querySelectorAll('.todo')) {
      todo.removeAttribute('hidden');
    }
  }
}

window.addEventListener('load', show_todos);
