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

// animate_placement builds and animates the placement graphic in the #placement
// SVG.
function animate_placement() {
  const w = 200;  // SVG width
  const h = 100;  // SVG height
  const mw = 40;  // machine width
  const mh = 35;  // machine height
  const cr = 7;   // component radius
  let svg = Snap('#placement');

  // Place machines.
  let mxys = [
    {x: 20, y: h / 2 - mh / 2},           // Machine 1
    {x: w / 2 - mw / 2, y: 10},           // Machine 2
    {x: w / 2 - mw / 2, y: h - mh - 10},  // Machine 3
  ];
  let machines = [];
  for (let {x, y} of mxys) {
    let m = svg.rect(x, y, mw, mh).attr({
      fill: '#272822',
      stroke: '#272822',
      strokeWidth: 1.25,
    });
    machines.push(m);
  }

  // Place machine labels.
  let labels = [
    {x: 40, y: 30, label: 'Machine 1'},
    {x: w / 2, y: 8, label: 'Machine 2'},
    {x: w / 2, y: h - 1, label: 'Machine 3'},
  ];
  for (let {x, y, label} of labels) {
    svg.text(x, y, label).attr({
      textAnchor: 'middle',
      class: 'component',
    })
  }

  // Compute slots, the places where components go. slot[i][j] is the slot for
  // the ith component in the jth machine.
  let slots = {0: [], 1: [], 2: [], 3: []};
  for (let m of machines) {
    let x = parseInt(m.attr('x'));
    let y = parseInt(m.attr('y'));
    slots[0].push({x: x + 11, y: y + 10});  // red component
    slots[1].push({x: x + 29, y: y + 10});  // blue component
    slots[2].push({x: x + 11, y: y + 26});  // green component
    slots[3].push({x: x + 29, y: y + 26});  // purple component replica 1
  }

  // Create components.
  let components = [];
  for (let i = 0; i < 4; i++) {
    let comp = svg.circle(slots[i][0].x, slots[i][0].y, cr);
    comp.attr({class: 'component-' + i});
    components.push(comp);
  }

  // Animate components.
  animate_component(svg, components[1], [slots[1][1], slots[1][0]]);
  animate_component(svg, components[3], [slots[3][2], slots[3][0]]);
}

// animate_component animates the movement of the provided component through the
// xy coordinates provided in xys.
function animate_component(svg, comp, xys) {
  let i = 0;
  let animate = function() {
    setTimeout(function() {
      comp.animate({cx: xys[i].x, cy: xys[i].y}, 2000, mina.easeinout, animate);
      i = (i + 1) % xys.length;
    }, 1000);
  };
  animate();
}

window.addEventListener('load', animate_placement);
