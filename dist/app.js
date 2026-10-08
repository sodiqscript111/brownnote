const $=id=>document.getElementById(id), editor=$('editor'),name=$('filename');
let previous='';
function persist(){try{localStorage.setItem('brownnote-draft',JSON.stringify({text:editor.value,name:name.value,spell:$('spell').checked,font:$('font').value}));$('saved').textContent='● Saved on this browser';}catch{$('saved').textContent='● Browser storage unavailable';}}
function update(){const text=editor.value;const before=text.slice(0,editor.selectionStart);$('counts').textContent=`${text.trim()?text.trim().split(/\s+/u).length:0} words  |  ${text.length} characters`;$('position').textContent=`Ln ${before.split('\n').length}, Col ${before.length-before.lastIndexOf('\n')}`;}
function font(){editor.style.fontFamily={mono:"'Courier New',monospace",serif:'Georgia,serif',sans:'Arial,sans-serif'}[$('font').value];}
try{const draft=JSON.parse(localStorage.getItem('brownnote-draft'));if(draft){editor.value=draft.text||'';name.value=draft.name||'untitled.txt';$('spell').checked=draft.spell!==false;$('font').value=draft.font||'mono';}}catch{}
editor.spellcheck=$('spell').checked;font();update();
editor.addEventListener('beforeinput',()=>previous=editor.value);editor.addEventListener('input',()=>{update();persist()});for(const event of ['keyup','click','select'])editor.addEventListener(event,update);name.addEventListener('input',persist);
$('spell').onchange=()=>{editor.spellcheck=$('spell').checked;persist();editor.focus()};$('font').onchange=()=>{font();persist()};
$('save').onclick=()=>{const url=URL.createObjectURL(new Blob([editor.value],{type:'text/plain;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download=(name.value.trim()||'untitled.txt').replace(/[\\/:*?"<>|]/g,'_');a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};
$('new').onclick=()=>{if(editor.value)$('confirm').showModal();else clear()};function clear(){previous=editor.value;editor.value='';name.value='untitled.txt';update();persist();editor.focus()};$('cancel').onclick=()=>$('confirm').close();$('clear').onclick=()=>{$('confirm').close();clear()};
$('open').onclick=()=>{if(editor.value)$('confirmOpen')?.remove();if(!editor.value||window.confirm('Open another file? Save your current text first if you want to keep it.'))$('file').click()};$('file').onchange=async()=>{const file=$('file').files[0];if(!file)return;previous=editor.value;editor.value=await file.text();name.value=file.name;update();persist();$('file').value='';editor.focus()};
$('undo').onclick=()=>{const current=editor.value;editor.value=previous;previous=current;update();persist();editor.focus()};
document.addEventListener('keydown',e=>{if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='s'){e.preventDefault();$('save').click()}});
