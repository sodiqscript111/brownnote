const $=id=>document.getElementById(id), editor=$('editor'),name=$('filename');
function persist(){try{localStorage.setItem('brownnote-draft',JSON.stringify({text:editor.value,name:name.value,spell:$('spell').checked,font:$('font').value}));$('saved').textContent='● Saved on this browser';}catch{$('saved').textContent='● Browser storage unavailable';}}
function update(){const text=editor.value;const before=text.slice(0,editor.selectionStart);$('counts').textContent=`${text.trim()?text.trim().split(/\s+/u).length:0} words  |  ${text.length} characters`;$('position').textContent=`Ln ${before.split('\n').length}, Col ${before.length-before.lastIndexOf('\n')}`;}
function font(){editor.style.fontFamily={mono:"'Courier New',monospace",serif:'Georgia,serif',sans:'Arial,sans-serif'}[$('font').value];}
try{const draft=JSON.parse(localStorage.getItem('brownnote-draft'));if(draft){editor.value=draft.text||'';name.value=draft.name||'untitled.txt';$('spell').checked=draft.spell!==false;$('font').value=draft.font||'mono';}}catch{}
editor.spellcheck=$('spell').checked;font();update();
editor.addEventListener('input',()=>{update();persist()});for(const event of ['keyup','click','select'])editor.addEventListener(event,update);name.addEventListener('input',persist);
$('spell').onchange=()=>{editor.spellcheck=$('spell').checked;persist();editor.focus()};$('font').onchange=()=>{font();persist()};
