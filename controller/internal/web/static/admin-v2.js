const csrf = document.querySelector('meta[name=csrf]')?.content || '';
const $ = selector => document.querySelector(selector);
const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
const state = {dashboard:{},users:[],groups:[],connections:[],credentials:[],devices:[],policies:[],permissions:[],audit:[]};
let toastTimer;
let lastActivity = Date.now();
for (const eventName of ['pointerdown','keydown','touchstart','scroll']) {
  window.addEventListener(eventName, () => { lastActivity = Date.now(); }, {passive:true});
}

async function api(path, options = {}) {
  options.headers = {...(options.headers || {}), 'Content-Type':'application/json', 'X-CSRF-Token':csrf};
  const response = await fetch('/api/v1/admin' + path, options);
  if (!response.ok) {
    const payload = await response.json().catch(() => ({error:{message:response.statusText}}));
    if (response.status === 401 || payload.error?.code === 'ADMIN_REQUIRED') {
      window.location.replace('/admin/login');
      return new Promise(() => {});
    }
    throw new Error(payload.error?.message || response.statusText);
  }
  return response.status === 204 ? null : response.json();
}

function note(message, bad = false) {
  const toast = $('#status');
  toast.textContent = message;
  toast.className = 'toast show' + (bad ? ' bad' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.className = 'toast', 3500);
}

function showView(name) {
  document.querySelectorAll('.nav-item,.view').forEach(element => element.classList.remove('active'));
  document.querySelector(`.nav-item[data-tab="${name}"]`)?.classList.add('active');
  $('#' + name)?.classList.add('active');
  window.scrollTo({top:0, behavior:'smooth'});
}

document.querySelectorAll('.nav-item').forEach(button => button.addEventListener('click', () => showView(button.dataset.tab)));
document.addEventListener('click', event => {
  const jump = event.target.closest('[data-jump]');
  if (jump) showView(jump.dataset.jump);
});

function badge(text, tone = '') { return `<span class="badge ${tone}">${esc(text)}</span>`; }
function credentialName(id) { return state.credentials.find(item => item.id === id)?.name || 'No stored credential'; }
function protocolLabel(protocol) { return ({rdp:'RDP',vnc:'Linux · VNC',ssh:'Linux · SSH',moonlight:'Moonlight',mock:'Demo'}[protocol] || protocol.toUpperCase()); }
function connectionPayload(connection, overrides={}) { return {name:connection.name,description:connection.description||'',protocol:connection.protocol,host:connection.host,port:Number(connection.port),enabled:!!connection.enabled,icon:connection.icon||'',sort_order:Number(connection.sort_order||0),protocol_config:JSON.parse(connection.protocol_config_json||'{}'),credential_id:connection.credential_id||null,...overrides}; }
function addPasswordToggle(input) { if(input.dataset.visibilityToggle)return;input.dataset.visibilityToggle='1';const wrapper=document.createElement('span'),button=document.createElement('button');wrapper.className='password-control';button.type='button';button.className='password-toggle';button.textContent='👁';button.setAttribute('aria-label','Show password');input.before(wrapper);wrapper.append(input,button);button.onclick=()=>{const show=input.type==='password';input.type=show?'text':'password';button.setAttribute('aria-label',show?'Hide password':'Show password');button.classList.toggle('revealed',show);input.focus();}; }
function policyFor(userID) { return state.policies.find(policy => policy.user_id === userID) || {allowed_days_mask:127,access_start_minute:0,access_end_minute:1440,daily_limit_minutes:0,max_session_minutes:0,idle_logout_minutes:30,timezone:'Australia/Sydney'}; }
function timeLabel(minutes) { if (minutes === 1440) return '24:00'; return `${String(Math.floor(minutes / 60)).padStart(2,'0')}:${String(minutes % 60).padStart(2,'0')}`; }
function policySummary(policy) {
  const schedule = policy.allowed_days_mask === 127 && policy.access_start_minute === 0 && policy.access_end_minute === 1440 ? 'Any time' : `${timeLabel(policy.access_start_minute)}–${timeLabel(policy.access_end_minute)}`;
  const allowance = policy.daily_limit_minutes ? `${policy.daily_limit_minutes} min/day` : 'No daily cap';
  const session = policy.max_session_minutes ? `${policy.max_session_minutes} min/session` : 'No session cap';
  return `${schedule} · ${allowance} · ${session} · ${policy.idle_logout_minutes || 30} min idle logout`;
}

function renderDashboard() {
  const cards = [
    ['◎', state.dashboard.enabled_users, 'Active people'],
    ['▰', state.dashboard.online_devices, 'Devices online'],
    ['▣', state.connections.filter(x => x.enabled).length, 'Connections'],
    ['↗', state.dashboard.launches_24h, 'Launches today'],
    ['!', state.dashboard.failed_logins_24h, 'Failed logins']
  ];
  $('#metrics').innerHTML = cards.map(([icon,value,label]) => `<article class="metric"><span class="metric-icon">${icon}</span><strong>${esc(value)}</strong><span>${esc(label)}</span></article>`).join('');
  const restricted = state.policies.filter(p => p.daily_limit_minutes || p.max_session_minutes || p.allowed_days_mask !== 127 || p.access_start_minute !== 0 || p.access_end_minute !== 1440);
  $('#policy-snapshot').innerHTML = restricted.length ? restricted.slice(0,4).map(policy => `<div class="policy-row"><span>${esc(policy.display_name)}</span><small>${esc(policySummary(policy))}</small></div>`).join('') : '<p class="muted">No restrictions have been configured.</p>';
}

function renderPeople() {
  $('#users-body').innerHTML = state.users.map(user => {
    const policy = policyFor(user.id);
    return `<tr><td><div class="person"><span class="avatar">${esc(user.display_name.slice(0,1).toUpperCase())}</span><div><strong>${esc(user.display_name)}</strong><small>@${esc(user.username)}</small></div></div></td><td>${user.is_admin ? badge('Administrator','blue') : badge('Member')}</td><td><small>${esc(policySummary(policy))}</small></td><td>${user.enabled ? badge('Active','green') : badge('Disabled','red')}</td><td><div class="actions"><button class="small-button" data-action="assign-user" data-id="${user.id}">Assign connections</button><button class="small-button" data-action="policy" data-id="${user.id}">Restrictions</button><button class="small-button" data-action="edit-user" data-id="${user.id}">Edit</button><button class="small-button ${user.enabled ? 'danger-button' : ''}" data-action="toggle-user" data-id="${user.id}">${user.enabled ? 'Disable' : 'Enable'}</button></div></td></tr>`;
  }).join('');
  $('#groups-list').innerHTML = state.groups.map(group => `<article class="list-card"><div class="list-card-main"><span class="list-icon">◎</span><div><h3>${esc(group.name)}</h3><p>${esc(group.description || 'No description')}</p></div></div><div class="actions"><button class="small-button" data-action="assign-group" data-id="${group.id}">Assign connections</button><button class="small-button" data-action="edit-group" data-id="${group.id}">Edit</button><button class="small-button danger-button" data-action="delete" data-path="/groups/${group.id}">Delete</button></div></article>`).join('');
}

function renderConnections() {
  $('#connections-list').innerHTML = state.connections.map(connection => `<article class="connection-card"><div class="connection-card-head"><span class="protocol-icon">${connection.protocol === 'vnc' ? 'L' : connection.protocol === 'ssh' ? '&gt;_' : connection.protocol === 'rdp' ? 'W' : connection.protocol === 'moonlight' ? 'M' : 'D'}</span>${connection.enabled ? badge('Available','green') : badge('Disabled','red')}</div><h3>${esc(connection.name)}</h3><p>${esc(connection.description || 'No description')}</p><div class="connection-meta">${badge(protocolLabel(connection.protocol),'blue')}${badge(`${connection.host}:${connection.port}`)}${badge(credentialName(connection.credential_id))}</div><div class="actions"><button class="small-button" data-action="assign-connection" data-id="${connection.id}">Assign access</button><button class="small-button" data-action="edit-connection" data-id="${connection.id}">Configure</button><button class="small-button" data-action="toggle-connection" data-id="${connection.id}">${connection.enabled ? 'Disable' : 'Enable'}</button><button class="small-button danger-button" data-action="delete" data-path="/connections/${connection.id}">Delete</button></div></article>`).join('');
}

function renderAccess() {
  $('#permissions-body').innerHTML = state.permissions.map(rule => `<tr><td>${badge(rule.subject_type === 'user' ? 'Person' : 'Group', rule.subject_type === 'user' ? 'blue' : '')} <strong>${esc(rule.subject_name)}</strong></td><td>${esc(rule.connection_name)}</td><td>${rule.credential_name ? badge(rule.credential_name,'green') : '<span class="muted">Connection default</span>'}</td><td>${rule.can_launch ? badge('Allowed','green') : badge('Blocked','red')} <button class="small-button danger-button" data-action="remove-assignment" data-subject-type="${rule.subject_type}" data-subject-id="${rule.subject_id}" data-connection-id="${rule.connection_id}">Remove</button></td></tr>`).join('');
  fillSelectors();
}

function renderDevices() {
  $('#devices-list').innerHTML = state.devices.map(device => `<article class="list-card"><div class="list-card-main"><span class="list-icon">▰</span><div><h3>${esc(device.name)}</h3><p>${esc(device.device_identifier)} · Last seen ${esc(device.last_seen_at || 'never')}</p></div></div><div class="actions">${device.enabled ? badge('Enrolled','green') : badge('Revoked','red')}<button class="small-button" data-action="rename-device" data-id="${device.id}">Rename</button><button class="small-button ${device.enabled ? 'danger-button' : ''}" data-action="toggle-device" data-id="${device.id}">${device.enabled ? 'Revoke' : 'Restore'}</button></div></article>`).join('');
}

function renderCredentials() {
  $('#credentials-list').innerHTML = state.credentials.map(credential => {
    const direct = state.connections.filter(c => c.credential_id === credential.id).length;
    const overrides = state.permissions.filter(p => p.credential_id === credential.id).length;
    return `<article class="list-card"><div class="list-card-main"><span class="list-icon">●</span><div><h3>${esc(credential.name)}</h3><p>${esc(credential.username || 'No username')} · Used by ${direct + overrides} assignment${direct + overrides === 1 ? '' : 's'}</p></div></div><div class="actions">${badge('Encrypted','green')}<button class="small-button" data-action="replace-credential" data-id="${credential.id}">Replace secret</button><button class="small-button danger-button" data-action="delete" data-path="/credentials/${credential.id}">Delete</button></div></article>`;
  }).join('');
}

function renderAudit() {
  const users = Object.fromEntries(state.users.map(user => [user.id,user.display_name]));
  const devices = Object.fromEntries(state.devices.map(device => [device.id,device.name]));
  const connections = Object.fromEntries(state.connections.map(connection => [connection.id,connection.name]));
  $('#audit-body').innerHTML = state.audit.map(event => `<tr data-search="${esc([event.event_type,event.result,users[event.actor_user_id],devices[event.device_id],connections[event.connection_id]].join(' ').toLowerCase())}"><td><small>${esc(new Date(event.timestamp).toLocaleString())}</small></td><td>${esc(event.event_type.replaceAll('_',' '))}</td><td>${esc(users[event.actor_user_id] || 'System')}</td><td>${esc(devices[event.device_id] || '—')}</td><td>${esc(connections[event.connection_id] || '—')}</td><td>${badge(event.result,event.result === 'success' ? 'green' : event.result === 'policy' ? 'yellow' : 'red')}</td></tr>`).join('');
}

function fillSelectors() {
  const selectedSubject=$('#perm-subject').value, selectedConnection=$('#perm-connection').value, selectedPermissionCredential=$('#perm-credential').value, selectedConnectionCredential=$('#connection-credential').value;
  const subjectType = $('#perm-subject-type').value;
  const subjects = subjectType === 'user' ? state.users : state.groups;
  $('#perm-subject').innerHTML = subjects.map(item => `<option value="${item.id}">${esc(item.display_name || item.name)}</option>`).join('');
  $('#perm-connection').innerHTML = state.connections.map(item => `<option value="${item.id}">${esc(item.name)}</option>`).join('');
  if([...$('#perm-subject').options].some(option=>option.value===selectedSubject))$('#perm-subject').value=selectedSubject;
  if([...$('#perm-connection').options].some(option=>option.value===selectedConnection))$('#perm-connection').value=selectedConnection;
  fillPermissionCredentialOptions(selectedPermissionCredential);
  fillConnectionCredentialOptions(selectedConnectionCredential);
  $('#member-user').innerHTML = state.users.map(item => `<option value="${item.id}">${esc(item.display_name)}</option>`).join('');
  $('#member-group').innerHTML = state.groups.map(item => `<option value="${item.id}">${esc(item.name)}</option>`).join('');
}

async function refresh() {
  try {
    const paths = ['/dashboard','/users','/groups','/connections','/credentials','/devices','/policies','/permissions','/audit?limit=100'];
    const [dashboard,users,groups,connections,credentials,devices,policies,permissions,audit] = await Promise.all(paths.map(path => api(path)));
    Object.assign(state,{dashboard,users:users.items,groups:groups.items,connections:connections.items,credentials:credentials.items,devices:devices.items,policies:policies.items,permissions:permissions.items,audit:audit.items});
    renderDashboard(); renderPeople(); renderConnections(); renderAccess(); renderDevices(); renderCredentials(); renderAudit();
  } catch (error) { note(error.message, true); }
}

function formData(form, mapping = {}) {
  const data = Object.fromEntries(new FormData(form));
  for (const [key, transform] of Object.entries(mapping)) data[key] = transform(data[key]);
  return data;
}

async function submit(form, path, mapping) {
  try {
    const data = formData(form, mapping);
    await api(path,{method:'POST',body:JSON.stringify(data)});
    form.reset(); await refresh(); note('Saved');
  } catch (error) { note(error.message,true); }
}

$('#user-form').onsubmit = event => { event.preventDefault(); submit(event.target,'/users',{is_admin:v=>v==='on',enabled:v=>v==='on'}); };
$('#group-form').onsubmit = event => { event.preventDefault(); submit(event.target,'/groups'); };
$('#credential-form').onsubmit = event => { event.preventDefault(); const data=formData(event.target); data.secret=data.secret_type==='ssh_private_key'?data.private_key:data.secret_type==='password'?data.password:''; delete data.password;delete data.private_key; submitCredential(event.target,data); };
$('#permission-form').onsubmit = event => { event.preventDefault(); submit(event.target,'/permissions',{subject_id:Number,connection_id:Number,credential_id:v=>v?Number(v):null,can_launch:v=>v==='on'}); };
$('#membership-form').onsubmit = event => { event.preventDefault(); submit(event.target,'/memberships',{user_id:Number,group_id:Number,member:v=>v==='on'}); };
$('#token-form').onsubmit = async event => { event.preventDefault(); try { const data=formData(event.target,{ttl_minutes:Number}); const result=await api('/enrolment-tokens',{method:'POST',body:JSON.stringify(data)}); $('#new-token').textContent=result.token; note('Enrolment token created'); } catch(error){note(error.message,true);} };
$('#connection-form').onsubmit = event => { event.preventDefault(); const data=formData(event.target,{port:Number,sort_order:Number,enabled:v=>v==='on',credential_id:v=>v?Number(v):null,protocol_config:v=>JSON.parse(v||'{}')}); if(data.protocol==='ssh')data.protocol_config={terminal_title:data.ssh_terminal_title.trim()||'Secure shell'};else if(data.protocol==='rdp')data.protocol_config={...data.protocol_config,certificate_mode:data.rdp_certificate_mode}; delete data.ssh_terminal_title;delete data.rdp_certificate_mode; submitData(event.target,'/connections',data); };
$('#perm-subject-type').onchange = fillSelectors;

const protocolDefaults = {
  rdp:{port:3389,config:{fullscreen:true,dynamic_resolution:true,audio:true,clipboard:false,certificate_mode:'tofu'}},
  vnc:{port:5900,config:{fullscreen:true,shared:true,view_only:false,clipboard:false}},
  ssh:{port:22,config:{terminal_title:'Secure shell'}},
  moonlight:{port:47984,config:{application:'Desktop',width:1920,height:1080,fps:60,bitrate_kbps:20000,audio:true,gamepad:true}},
  mock:{port:1,config:{}}
};
$('#connection-protocol').onchange = event => { const defaults=protocolDefaults[event.target.value]; $('#connection-form [name=port]').value=defaults.port; $('#connection-form [name=protocol_config]').value=JSON.stringify(defaults.config,null,2); syncConnectionFields(); fillConnectionCredentialOptions(''); };

async function submitData(form,path,data){try{const result=await api(path,{method:'POST',body:JSON.stringify(data)});form.reset();syncConnectionFields();syncCredentialFields();await refresh();if(path==='/connections'&&result?.id){showView('access');$('#perm-connection').value=String(result.id);fillPermissionCredentialOptions('');note('Connection saved — now assign it to a person or group');}else note('Saved');}catch(error){note(error.message,true);}}
async function submitCredential(form,data){return submitData(form,'/credentials',data);}
function credentialsForProtocol(protocol){return state.credentials.filter(item=>item.secret_type!=='ssh_private_key'||protocol==='ssh');}
function optionsForCredentials(items){return items.map(item=>`<option value="${item.id}">${esc(item.name)} · ${esc(item.username||'no username')}</option>`).join('');}
function fillConnectionCredentialOptions(selected=$('#connection-credential').value){const select=$('#connection-credential'),items=credentialsForProtocol($('#connection-protocol').value);select.innerHTML='<option value="">No stored credential</option>'+optionsForCredentials(items);if([...select.options].some(option=>option.value===String(selected)))select.value=String(selected);}
function fillPermissionCredentialOptions(selected=$('#perm-credential').value){const connection=state.connections.find(item=>String(item.id)===$('#perm-connection').value),items=credentialsForProtocol(connection?.protocol||'');const select=$('#perm-credential');select.innerHTML='<option value="">Use connection default</option>'+optionsForCredentials(items);if([...select.options].some(option=>option.value===String(selected)))select.value=String(selected);}
function syncConnectionFields(){const protocol=$('#connection-protocol').value,ssh=protocol==='ssh';$('#ssh-title-field').hidden=!ssh;$('#rdp-certificate-field').hidden=protocol!=='rdp';$('#protocol-config-field').hidden=ssh;}
function syncCredentialFields(){const type=$('#credential-type').value;$('#credential-password-field').hidden=type!=='password';$('#credential-key-field').hidden=type!=='ssh_private_key';$('#credential-form [name=password]').required=type==='password';$('#credential-form [name=private_key]').required=type==='ssh_private_key';}
$('#credential-type').onchange=syncCredentialFields;
$('#perm-connection').onchange=()=>fillPermissionCredentialOptions('');

function ask(title, fields = [], confirmLabel = 'Save changes', message = '') {
  return new Promise(resolve => {
    const dialog=$('#editor-dialog'), form=$('#editor-form'), container=$('#editor-fields');
    $('#editor-title').textContent=title; $('#editor-message').textContent=message; $('#editor-message').hidden=!message; $('#editor-submit').textContent=confirmLabel; container.replaceChildren();
    for (const field of fields) {
      const label=document.createElement('label');
      if (field.type === 'checkbox') label.className='check';
      const input=field.options ? document.createElement('select') : document.createElement(field.multiline ? 'textarea' : 'input');
      input.name=field.name;
      if (field.options) { for (const option of field.options) input.add(new Option(option.label,option.value)); input.value=String(field.value??''); }
      else if (field.type === 'checkbox') { input.type='checkbox'; input.checked=!!field.value; }
      else { input.type=field.type||'text'; input.value=field.value??''; }
      input.required=!!field.required;
      if(field.min!==undefined)input.min=field.min;if(field.max!==undefined)input.max=field.max;if(field.minLength!==undefined)input.minLength=field.minLength;
      if(field.type==='checkbox')label.append(input,document.createTextNode(field.label));else label.append(document.createTextNode(field.label),input); container.append(label); if(input.type==='password')addPasswordToggle(input);
    }
    const syncConditionalFields=()=>{for(const field of fields){if(!field.showWhen)continue;const current=form.elements[field.showWhen.field]?.value;form.elements[field.name]?.closest('label')?.toggleAttribute('hidden',!field.showWhen.values.includes(current));}};
    for(const field of fields.filter(item=>item.showWhen))form.elements[field.showWhen.field]?.addEventListener('change',syncConditionalFields);
    syncConditionalFields();
    let settled=false; const finish=value=>{if(settled)return;settled=true;resolve(value)};
    const cancel=()=>{dialog.close();finish(null)}; $('#editor-cancel').onclick=cancel; $('#editor-close').onclick=cancel; dialog.oncancel=event=>{event.preventDefault();cancel()};
    form.onsubmit=event=>{event.preventDefault();if(!form.reportValidity())return;const values=Object.fromEntries(new FormData(form));for(const field of fields.filter(x=>x.type==='checkbox'))values[field.name]=form.elements[field.name].checked;dialog.close();finish(values)};
    dialog.showModal(); container.querySelector('input,select,textarea')?.focus();
  });
}

async function update(path, body, message='Updated') { try { await api(path,{method:'PUT',body:JSON.stringify(body)}); await refresh(); note(message); } catch(error){note(error.message,true);} }
async function removeItem(path) { const answer=await ask('Delete item',[],'Delete','This cannot be undone.'); if(answer===null)return; try{await api(path,{method:'DELETE'});await refresh();note('Deleted');}catch(error){note(error.message,true);} }

function credentialOptions(selected,protocol) { return [{label:'No stored credential',value:''},...credentialsForProtocol(protocol).map(item=>({label:`${item.name} · ${item.username||'no username'}`,value:String(item.id)}))].map(option=>({...option,value:option.value,label:option.label})); }

async function handleAction(button) {
  const id=Number(button.dataset.id), action=button.dataset.action;
  const user=state.users.find(x=>x.id===id), connection=state.connections.find(x=>x.id===id), group=state.groups.find(x=>x.id===id), credential=state.credentials.find(x=>x.id===id), device=state.devices.find(x=>x.id===id);
  if(action==='delete') return removeItem(button.dataset.path);
  if(action==='assign-user'){showView('access');$('#perm-subject-type').value='user';fillSelectors();$('#perm-subject').value=String(id);$('#perm-connection').focus();return;}
  if(action==='assign-group'){showView('access');$('#perm-subject-type').value='group';fillSelectors();$('#perm-subject').value=String(id);$('#perm-connection').focus();return;}
  if(action==='assign-connection'){showView('access');$('#perm-connection').value=String(id);fillPermissionCredentialOptions('');$('#perm-subject').focus();return;}
  if(action==='remove-assignment'){const answer=await ask('Remove access',[],'Remove','This person or group will no longer be able to launch the connection.');if(answer===null)return;try{await api('/permissions',{method:'POST',body:JSON.stringify({subject_type:button.dataset.subjectType,subject_id:Number(button.dataset.subjectId),connection_id:Number(button.dataset.connectionId),can_launch:false,credential_id:null})});await refresh();note('Access removed');}catch(error){note(error.message,true);}return;}
  if(action==='toggle-user') return update('/users/'+id,{display_name:user.display_name,is_admin:user.is_admin,enabled:!user.enabled,password:''},user.enabled?'Account disabled':'Account enabled');
  if(action==='edit-user') { const data=await ask('Edit person',[{name:'display_name',label:'Display name',value:user.display_name,required:true},{name:'password',label:'New password (leave blank to keep current)',type:'password',minLength:8},{name:'is_admin',label:'Administrator',type:'checkbox',value:user.is_admin},{name:'enabled',label:'Active account',type:'checkbox',value:user.enabled}]); if(data)return update('/users/'+id,data); }
  if(action==='policy') return openPolicy(id);
  if(action==='edit-group') { const data=await ask('Edit group',[{name:'name',label:'Group name',value:group.name,required:true},{name:'description',label:'Description',value:group.description||'',multiline:true}]); if(data)return update('/groups/'+id,data); }
  if(action==='toggle-connection') return update('/connections/'+id,connectionPayload(connection,{enabled:!connection.enabled}),connection.enabled?'Connection disabled':'Connection enabled');
  if(action==='edit-connection') {
    const protocols=[{label:'Windows / Linux RDP',value:'rdp'},{label:'Linux desktop (VNC)',value:'vnc'},{label:'Linux command line (locked SSH)',value:'ssh'},{label:'Moonlight / Sunshine',value:'moonlight'}];
    if(state.dashboard.dev_mode)protocols.push({label:'Demo session',value:'mock'});
    const config=JSON.parse(connection.protocol_config_json||'{}');
    const data=await ask('Configure connection',[{name:'name',label:'Name',value:connection.name,required:true},{name:'description',label:'Description',value:connection.description||''},{name:'protocol',label:'Type',value:connection.protocol,options:protocols},{name:'host',label:'Host',value:connection.host,required:true},{name:'port',label:'Port',type:'number',value:connection.port,min:1,max:65535,required:true},{name:'credential_id',label:'Default credential',value:String(connection.credential_id||''),options:credentialOptions(connection.credential_id,connection.protocol)},{name:'enabled',label:'Available',type:'checkbox',value:connection.enabled},{name:'rdp_certificate_mode',label:'RDP certificate validation',value:config.certificate_mode||'tofu',options:[{label:'Trust on first connection, then pin',value:'tofu'},{label:'Require already trusted certificate',value:'deny'},{label:'Ignore validation (unsafe)',value:'ignore'}],showWhen:{field:'protocol',values:['rdp']}},{name:'ssh_terminal_title',label:'Terminal title',value:config.terminal_title||'Secure shell',showWhen:{field:'protocol',values:['ssh']}},{name:'protocol_config',label:'Advanced settings (JSON)',value:connection.protocol_config_json||'{}',multiline:true,showWhen:{field:'protocol',values:['rdp','vnc','moonlight','mock']}}]);
    if(data){const parsed=JSON.parse(data.protocol_config),protocolConfig=data.protocol==='ssh'?{terminal_title:data.ssh_terminal_title.trim()||'Secure shell'}:data.protocol==='rdp'?{...parsed,certificate_mode:data.rdp_certificate_mode}:parsed;return update('/connections/'+id,connectionPayload(connection,{name:data.name,description:data.description,protocol:data.protocol,host:data.host,port:Number(data.port),credential_id:data.credential_id?Number(data.credential_id):null,protocol_config:protocolConfig,enabled:data.enabled}));}
  }
  if(action==='replace-credential') { const isKey=credential.secret_type==='ssh_private_key';const data=await ask('Replace credential',[{name:'username',label:'Remote username (blank preserves current)',value:credential.username||''},{name:'secret',label:isKey?'New SSH private key':'New password',type:isKey?'text':'password',multiline:isKey,required:true}]); if(data)return update('/credentials/'+id,data,'Credential replaced'); }
  if(action==='rename-device') { const data=await ask('Rename device',[{name:'name',label:'Device name',value:device.name,required:true}]); if(data)return update('/devices/'+id,{name:data.name}); }
  if(action==='toggle-device') { if(device.enabled){const answer=await ask('Revoke device',[],'Revoke','It will no longer be able to launch sessions.');if(answer===null)return;} return update('/devices/'+id,{enabled:!device.enabled},device.enabled?'Device revoked':'Device restored'); }
}

document.addEventListener('click', event => { const button=event.target.closest('button[data-action]'); if(button) handleAction(button); });

const dayNames=['Mon','Tue','Wed','Thu','Fri','Sat','Sun'];
let policyUserID=0;
function openPolicy(userID) {
  policyUserID=userID; const user=state.users.find(x=>x.id===userID), policy=policyFor(userID);
  $('#policy-title').textContent=`${user.display_name}'s access policy`;
  $('#policy-days').innerHTML=dayNames.map((day,index)=>`<label><input type="checkbox" value="${1<<index}" ${(policy.allowed_days_mask&(1<<index))?'checked':''}>${day}</label>`).join('');
  $('#policy-start').value=timeLabel(policy.access_start_minute); $('#policy-end').value=policy.access_end_minute===1440?'23:59':timeLabel(policy.access_end_minute); $('#policy-daily').value=policy.daily_limit_minutes; $('#policy-session').value=policy.max_session_minutes; $('#policy-idle').value=policy.idle_logout_minutes||30; $('#policy-timezone').value=policy.timezone;
  $('#policy-dialog').showModal();
}
function closePolicy(){ $('#policy-dialog').close(); }
document.querySelectorAll('[data-policy-close]').forEach(button=>button.onclick=closePolicy);
$('#policy-form').onsubmit=async event=>{event.preventDefault();let mask=0;document.querySelectorAll('#policy-days input:checked').forEach(input=>mask|=Number(input.value));const parseTime=value=>{const [h,m]=value.split(':').map(Number);return h*60+m};const end=$('#policy-end').value;const body={user_id:policyUserID,timezone:$('#policy-timezone').value,allowed_days_mask:mask,access_start_minute:parseTime($('#policy-start').value),access_end_minute:end==='23:59'?1440:parseTime(end),daily_limit_minutes:Number($('#policy-daily').value||0),max_session_minutes:Number($('#policy-session').value||0),idle_logout_minutes:Number($('#policy-idle').value||30)};closePolicy();await update('/policies/'+policyUserID,body,'Access policy saved');};

$('#audit-filter').oninput=event=>{const query=event.target.value.trim().toLowerCase();document.querySelectorAll('#audit-body tr').forEach(row=>row.hidden=!!query&&!row.dataset.search.includes(query));};

$('#return-client').onclick=async event=>{
  const button=event.currentTarget;
  button.disabled=true;
  button.textContent='Returning…';
  try {
    await fetch('/api/v1/auth/logout',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:'{}'});
  } catch (_) {}
  window.close();
  window.setTimeout(()=>{
    button.disabled=false;
    button.textContent='Close with Alt+F4';
    note('Administration signed out. Press Alt+F4 to return to ThinPi.');
  },500);
};

document.querySelectorAll('input[type=password]').forEach(addPasswordToggle);
syncConnectionFields();
syncCredentialFields();
refresh();
// Refresh only while the administrator has been active recently. Each API
// request slides the server session; an abandoned browser therefore expires.
setInterval(() => {
  if (!document.hidden && Date.now() - lastActivity < 90000) refresh();
},30000);
