(function(){let e=document.createElement(`link`).relList;if(e&&e.supports&&e.supports(`modulepreload`))return;for(let e of document.querySelectorAll(`link[rel="modulepreload"]`))n(e);new MutationObserver(e=>{for(let t of e)if(t.type===`childList`)for(let e of t.addedNodes)e.tagName===`LINK`&&e.rel===`modulepreload`&&n(e)}).observe(document,{childList:!0,subtree:!0});function t(e){let t={};return e.integrity&&(t.integrity=e.integrity),e.referrerPolicy&&(t.referrerPolicy=e.referrerPolicy),t.credentials=e.crossOrigin===`use-credentials`?`include`:e.crossOrigin===`anonymous`?`omit`:`same-origin`,t}function n(e){if(e.ep)return;e.ep=!0;let n=t(e);fetch(e.href,n)}})();function e(e){let t=[],n=``,r=-1,i=``,a=!1,o=e=>{r<0||(t.push({value:n,start:r,end:e}),n=``,r=-1)};for(let t=0;t<e.length;t+=1){let s=e[t];a?(n+=s,a=!1):s===`\\`?(r<0&&(r=t),a=!0):i?s===i?i=``:n+=s:s===`"`||s===`'`?(r<0&&(r=t),i=s):/\s/.test(s)?o(t):(r<0&&(r=t),n+=s)}return a&&(n+=`\\`),o(e.length),t}function t(t){return e(t).map(e=>e.value)}function n(e){return/[\s"'\\]/.test(e)?`"${e.replace(/["\\]/g,e=>`\\${e}`)}"`:e}var r={api:{overview:`api <命令>      模块 HTTP 管理接口`,actions:[`init`,`start`,`status`,`access`],help:`api - 模块 HTTP 管理接口

  api init      初始化独立随机凭据，不覆盖已有配置
  api start     启动或恢复后台管理接口
  api status    查看实际接口就绪状态
  api access    读取地址与凭据，仅供 Root 客户端连接

接口默认仅监听 127.0.0.1:9091，关闭 sing-box 不关闭管理接口。
凭据不可分享或粘贴到公共聊天中。
`},service:{overview:`service <命令>  服务控制`,actions:[`status`,`start`,`stop`,`restart`,`reload`,`check`,`toggle`],help:`service - 服务控制

  service status      查看服务状态、运行时间和流量
  service start       启动 sing-box 服务
  service stop        停止 sing-box 服务
  service restart     重启 sing-box 核心
  service reload      重载配置
  service check       检查服务配置
  service toggle      切换服务状态

状态: stopped / preparing / starting / ready / stopping / failed
configured_backend 是保存的入站；active_backend 仅在 ready 且 PID/API 毫秒启动身份一致时非空，否则为 null。
`},catalog:{overview:`catalog <命令>  分组查看`,actions:[`list`,`show`],help:`catalog - 节点分组

  catalog list             列出所有节点分组
  catalog show <分组>      查看分组详情
`},node:{overview:`node <命令>     节点管理`,actions:[`list`,`snapshot`,`current`,`show`,`get`,`export`,`delay`,`add`,`import`,`edit`,`remove`,`use`],help:`node - 节点管理

  node list [分组]                 列出节点
  node snapshot [分组]             查看运行时节点快照
  node current                     查看当前选择
  node show <分组>                 查看分组节点
  node get <分组>/<tag>            获取节点配置摘要
  node export <分组>/<tag>         导出节点链接
  node delay [目标] [分组]         测量节点延迟
  node delay auto <分组>           测量分组内所有节点
  node add <链接> [分组]           添加单节点链接
  node import <文件>               导入节点文件
  node edit <分组>/<tag> <来源>    编辑节点
  node remove <分组>/<tag>         删除节点
  node use auto [分组]             自动选择最快节点
  node use <分组>/<tag>            手动选择节点
`},sub:{overview:`sub <命令>      订阅管理`,actions:[`list`,`show`,`add`,`edit`,`update`,`update-all`,`activate`,`remove`,`history`,`cancel`],help:`sub - 订阅管理

  sub list                         列出订阅
  sub show <名称>                  查看订阅详情
  sub add [名称] <URL>             添加订阅
  sub edit <名称>                  编辑订阅
  sub update <名称>                更新单个订阅
  sub update-all                   更新所有订阅
  sub activate <名称>              激活订阅分组
  sub remove <名称>                删除订阅
  sub history <名称>               查看更新历史
  sub cancel <名称>                取消更新任务
`},mode:{overview:`mode <模式>     出站模式`,actions:[],help:`mode - 出站模式

  mode            查看默认模式、实际模式与可选模式
  mode <模式名称> 保存主配置默认模式，运行时应用网络策略

可选模式来自主配置的路由与 DNS 规则，名称与内核一致。
默认配置：Rule（规则）、Proxy（代理）、Direct（直连）、RuleAllowAds（规则，允许广告）。
示例：mode Proxy、mode RuleAllowAds。
`},network:{overview:`network <命令>  网络策略评估`,actions:[`evaluate`,`wifi-list`],help:`network - 网络策略

  network evaluate --type <类型> [--ssid <名称>]
                         评估当前网络并应用 Wi-Fi 策略
  network wifi-list       查看已保存的 Wi-Fi 名称
  类型: wifi / not_wifi
`},app:{overview:`app <命令>      分应用代理`,actions:[`list`,`mode`,`add`,`remove`,`enable`,`disable`],help:`app - 分应用代理

  app list                    查看应用代理配置
  app mode blacklist          使用黑名单模式
  app mode whitelist          使用白名单模式
  app add <用户ID>:<包名>      添加指定用户的应用
  app remove <用户ID>:<包名>   移除指定用户的应用
  app enable                  启用分应用代理
  app disable                 禁用分应用代理

共用名单保存在 inbound.json 的 app 对象，运行时修改自动应用；只筛选本机应用。
`},ebpf:{overview:`ebpf <命令>     eBPF 能力诊断`,actions:[`status`],help:`ebpf - eBPF 能力诊断

  ebpf status                         查看已配置能力
  ebpf status configured              查看当前配置
  ebpf status all                     检查全部支持能力
  ebpf status local                   检查本机数据路径
  ebpf status shared                  检查共享网络数据路径
  ebpf status all --raw               输出原始诊断信息

只诊断 eBPF 能力，不代表实际 active_backend；TUN 通过配置检查、实际启动与日志确认。
`},config:{overview:`config <命令>   配置管理`,actions:[`list`,`read`,`check`,`validate`,`apply`],help:`config - 配置管理

  config list                         列出配置文件
  config read <目标>                  读取配置
  config check                        检查全部配置
  config validate <目标> <内容文件>   校验配置
  config apply <目标> <内容文件>      应用配置

  完整配置：module（module.json）、singbox/config.json、inbound
  模块分区：module/wifi、module/auto_start
  模块分区保留顶层字段，例如 {"auto_start":true}、{"wifi":{...}}；不能用 {} 删除。
  模块分区 revision 独立，保存时保留 selection 和其他字段；auto_start 只影响下次开机。
  入站分区：inbound/backend、inbound/root_policy、inbound/app、inbound/ebpf、inbound/tun
  入站分区保留顶层字段，例如 {"backend":"tun"}；不能用 {} 删除。
  核心分区：singbox/dns、singbox/inbounds、singbox/route 等
  核心分区保留顶层字段，例如 {"dns":{...}}；{} 删除该分区。
  只读运行时：runtime/inbound.json、runtime/providers.json、runtime/outbounds.json
  apply/validate 在目标前加 --revision <读取值>，避免覆盖并发修改。
  切换强杀或清理未确认时中止并保留 journal，需要设备重启后再恢复。
`},logs:{overview:`logs <命令>     日志管理`,actions:[`show`,`clear`,`export`],help:`logs - 日志管理

  logs show service             查看服务日志
  logs show core                查看 sing-box 日志
  logs clear <类型>             清空日志
  logs export                   导出运行时配置和脱敏日志
`}},i=Object.keys(r),a=[...i,`help`,`clear`,`exit`],o=[...i,`shell`],s=[`module`,`module/wifi`,`module/auto_start`,`inbound`,`inbound/backend`,`inbound/root_policy`,`inbound/app`,`inbound/ebpf`,`inbound/tun`,`singbox/config.json`,`singbox/log`,`singbox/dns`,`singbox/inbounds`,`singbox/outbounds`,`singbox/route`,`singbox/experimental`,`singbox/http_clients`,`singbox/services`],c={"app mode":[`blacklist`,`whitelist`],"ebpf status":[`configured`,`all`,`local`,`shared`,`--raw`],"logs show":[`service`,`core`],"logs clear":[`service`,`core`],"network evaluate":[`--type`,`--ssid`],"network evaluate --type":[`wifi`,`not_wifi`],"config read":[...s,`runtime/inbound.json`,`runtime/providers.json`,`runtime/outbounds.json`],"config apply":s,"config validate":s},l=[`list`,`show`,`get`,`export`,`edit`,`remove`],ee=[`activate`,`update`,`show`,`edit`,`remove`,`history`,`cancel`];function te(t,r){let i=e(t),a=i[i.length-1],o=a?.end===t.length?a:void 0;return(o?t.slice(0,o.start):t)+n(r)+` `}function ne(e){let t=e[0]||``;for(let n=1;n<e.length&&t;n++)for(;!e[n].startsWith(t)&&t;)t=t.slice(0,-1);return t}function u(t,i=[],s=[],te=[]){if(t.startsWith(`!`))return{completed:t,candidates:[]};let u=e(t),d=u.map(e=>e.value),f=u.length>0&&u[u.length-1].end<t.length,p=f?d.length:Math.max(0,d.length-1),m=f?void 0:u[u.length-1],h=m?.value||``,g=[];if(p===0)g=a.filter(e=>e.startsWith(h));else if(p===1)g=(d[0]===`help`?o:d[0]===`mode`?te:r[d[0]]?.actions||[]).filter(e=>e.startsWith(h));else if(p>=2){let[e,t]=d;e===`node`&&(t===`use`||t===`delay`)?p===2?g=h===`auto`?i.filter(e=>e.startsWith(h)):[`auto`,...i].filter(e=>e.startsWith(h)):p===3&&d[2]===`auto`&&(g=i.filter(e=>e.startsWith(h))):g=e===`sub`&&ee.includes(t)&&p===2?s.filter(e=>e.startsWith(h)):e===`catalog`&&t===`show`&&p===2||e===`node`&&l.includes(t)&&p===2?i.filter(e=>e.startsWith(h)):(c[d.slice(0,p).join(` `)]||[]).filter(e=>e.startsWith(h))}if(!g.length)return{completed:t,candidates:[]};let _=ne(g),v=m?t.slice(0,m.start):t;return{completed:g.length===1?v+n(g[0])+` `:v+n(_),candidates:g}}function d(e){let t=e.out.trim();if(t)try{let n=JSON.parse(t);if(n.schema===1&&typeof n.ok==`boolean`&&typeof n.code==`string`&&typeof n.message==`string`)return!n.ok||e.code===0?n:{...n,ok:!1,code:`transport.failed`,message:e.err.trim()||`模块命令失败（退出码 ${e.code}）`}}catch{}return e.code===0?{schema:1,ok:!1,code:t?`transport.invalid_json`:`transport.empty`,message:t?`模块返回的数据格式无效`:e.err.trim()||`模块没有返回有效结果`}:{schema:1,ok:!1,code:`transport.failed`,message:e.err.trim()||`模块命令失败（退出码 ${e.code}）`}}var f=0;function p(e){return`${e}_callback_${Date.now()}_${f++}`}function m(e,t){return t===void 0&&(t={}),new Promise((n,r)=>{let i=p(`exec`);window[i]=(e,t,r)=>{n({errno:e,stdout:t,stderr:r}),a(i)};function a(e){delete window[e]}try{ksu.exec(e,JSON.stringify(t),i)}catch(e){r(e),a(i)}})}function h(){this.listeners={}}h.prototype.on=function(e,t){this.listeners[e]||(this.listeners[e]=[]),this.listeners[e].push(t)},h.prototype.emit=function(e,...t){this.listeners[e]&&this.listeners[e].forEach(e=>e(...t))};function g(){this.listeners={},this.stdin=new h,this.stdout=new h,this.stderr=new h}g.prototype.on=function(e,t){this.listeners[e]||(this.listeners[e]=[]),this.listeners[e].push(t)},g.prototype.emit=function(e,...t){this.listeners[e]&&this.listeners[e].forEach(e=>e(...t))};var _={auto_update:!1,update_interval:0,update_via_proxy:`auto`,usage:null,profile_title:``,profile_web_page_url:``,last_attempt_at:``,last_success_at:``,next_update_at:``,last_error:``,updated_at:``,progress:null};({..._}),{..._};var v={log:{level:`info`},dns:{final:`dns-proxy`},inbounds:[],route:{final:`Proxy`},experimental:{},http_clients:[],services:[]};[...[`wifi`,`auto_start`].map(e=>({id:`module/${e}`,filename:e,category:`module`,editable:!0,section:e}))],[...[`backend`,`root_policy`,`app`,`ebpf`,`tun`].map(e=>({id:`inbound/${e}`,filename:e,category:`inbound`,editable:!0,section:e})),...[...Object.keys(v),`outbounds`].map(e=>({id:`singbox/${e}`,filename:e,category:`config`,editable:!0,section:e})),...[`inbound.json`,`providers.json`,`outbounds.json`].map(e=>({id:`runtime/${e}`,filename:e,category:`runtime`,editable:!1}))];var re=`/data/adb/modules/netproxy/netproxyctl`,ie=3e4,y=typeof window>`u`?void 0:window,ae=typeof ksu<`u`||!!y?.ksu||!!y?.KSU,oe=e=>`'${e.replace(/'/g,`'"'"'`)}'`;async function b(e){if(!ae)return{out:``,err:`[非 KernelSU 环境]
请在 KernelSU WebUI 中打开此页面执行命令。`,code:1};try{let t=await m(e);return{out:t.stdout,err:t.stderr,code:t.errno}}catch(e){return{out:``,err:e?.message||String(e),code:-1}}}async function x(e){return b([re,...e].map(oe).join(` `))}async function se(e){return x(e)}async function ce(e,t=ie){return d(await x([`--json`,`--timeout`,`${Math.max(1,Math.ceil(t/1e3))}s`,...e]))}var le=b;function ue(e){if(typeof e!=`object`||!e)return!1;let t=e;return typeof t.id==`string`&&t.id.length>0&&typeof t.type==`string`}async function de(){let e=await ce([`catalog`,`list`]),t=e.ok&&Array.isArray(e.data)?e.data.filter(ue):[];return{groups:t.map(e=>e.id),subs:t.filter(e=>e.type===`subscription`).map(e=>e.id)}}function fe(e){let t=e.trim();if(!t)return e;try{let n=JSON.parse(t);return typeof n!=`object`||!n?e:`${JSON.stringify(n,null,2)}\n`}catch{return e}}var pe=`NetProxy Terminal
`,me=`
命令:
${i.map(e=>`  ${r[e].overview}`).join(`
`)}

全局选项:
  --json                 输出 schema=1 JSON
  --timeout <秒|时长>    覆盖默认命令超时

本地命令:
  help [主题]       查看帮助
  clear             清空终端
  exit              显示退出当前终端的提示
  ! <命令>          执行 Root Shell 命令

示例:
  service start
  node use auto default
  mode Rule
  mode Proxy
  sub update-all
  node delay auto default

输入 help all 查看完整命令说明。
`,S=`shell - WebUI 本地扩展

  ! <命令>                      执行 Root Shell 命令

此功能不属于 netproxyctl 公共命令，只在 KernelSU WebUI 中可用。
`;function he(e){return e===`shell`?S:r[e]?.help}function ge(e){if(!e)return pe+me;let t=e.toLowerCase();return t===`all`?pe+me+`
`+[...i.map(e=>r[e].help),S].join(`
`):he(t)||`未知主题: ${e}\n可用主题: ${o.join(`, `)}\n输入 help 查看用法。`}function _e(e,t,n=5e3){let r=!1,i=!1,a=!1,o=0,s;function c(){if(o++,clearTimeout(s),!r)return;if(i){a=!0;return}i=!0,a=!1;let l=o;e().then(e=>{r&&l===o&&t(e)}).catch(()=>{}).finally(()=>{i=!1,r&&(a?c():s=setTimeout(c,n))})}return{refresh:c,setActive(e){r!==e&&(r=e,o++,clearTimeout(s),r&&c())}}}var ve=`./mascots/dragon/dig-raised.svg`,ye=`./mascots/dragon/dig-strike.svg`,be=`./mascots/dragon/sleep-a.svg`,xe=`./mascots/dragon/sleep-b.svg`,Se=`./mascots/dragon/failed-a.svg`,Ce=`./mascots/dragon/failed-b.svg`,C=`./mascots/dragon/waiting-a.svg`,w=`./mascots/dragon/waiting-b.svg`,we={ready:{label:`服务运行中`,first:ve,second:ye},stopped:{label:`服务已停止`,first:be,second:xe},failed:{label:`服务启动失败`,first:Se,second:Ce},preparing:{label:`服务准备中`,first:C,second:w},starting:{label:`服务启动中`,first:C,second:w},stopping:{label:`服务停止中`,first:C,second:w}};function T(e){let t=document.getElementById(e);if(!t)throw Error(`缺少页面元素: ${e}`);return t}var E=T(`output`),Te=T(`welcome`),D=T(`command-form`),O=T(`command-input`),k=T(`suggestions`),Ee=T(`complete`),De=T(`history-prev`),Oe=T(`history-next`),ke=T(`run`),A=T(`copy`),j=T(`clear`),M=T(`latest`),N=T(`mascot`),Ae=T(`mascot-action`),je=T(`mascot-impact`),Me=T(`announcement`),P=T(`service-state`),Ne=T(`command-entry`),F=[],I=-1,L=``,R=!1,z=!1,B=!0,V=0,H=``,U=[],Pe=[],Fe=[],Ie=0,Le=``,Re=``;function W(e){Me.textContent=e}function ze(e,t,n){return e.hidden=!t,t&&t!==n&&(e.src=t),t??n}function Be(e,t){let n=e?.label??`服务状态不可用`;P.textContent!==n&&(P.textContent=n),t?N.dataset.state=t:delete N.dataset.state,e?N.dataset.animated=`true`:delete N.dataset.animated,Le=ze(Ae,e?.first,Le),Re=ze(je,e?.second,Re)}function G(){ke.disabled=R||!O.value.trim(),Ee.disabled=R,De.disabled=R||!F.length||I===0,Oe.disabled=R||I===-1,j.disabled=R||!E.querySelector(`.entry`),A.disabled=!H}function K(e=!1){e&&(B=!0),V&&cancelAnimationFrame(V),V=requestAnimationFrame(()=>{V=0,B&&(E.scrollTop=E.scrollHeight),M.hidden=E.scrollHeight-E.clientHeight-E.scrollTop<24})}function q(){k.hidden=!0,k.replaceChildren()}function J(){O.focus({preventScroll:!0}),O.setSelectionRange(O.value.length,O.value.length)}function Ve(e){O.value=e,I=-1,q(),G(),J()}async function He(){let e=++Ie;try{let t=await de();if(e!==Ie)return;U=t.groups,Pe=t.subs}catch{}}var Y=_e(()=>ce([`service`,`status`]),e=>{let t=e.ok?e.data?.state:void 0;Be(t?we[t]:void 0,t),e.ok&&Array.isArray(e.data?.available_outbound_modes)&&(Fe=e.data.available_outbound_modes)});function X(){E.replaceChildren(Te),H=``,A.textContent=`复制结果`,M.hidden=!0,B=!0,q(),G(),W(`终端输出已清空`)}function Z(e,t,n){n&&e.append(Object.assign(document.createElement(`pre`),{className:t,textContent:n}))}async function Ue(e){let n=e.trim();if(!n||R||z)return;if(F[F.length-1]!==n&&F.push(n),F.length>100&&F.shift(),I=-1,O.value=``,L=``,q(),n===`clear`){X();return}let r=Ne.content.firstElementChild.cloneNode(!0);Object.assign(r.querySelector(`.entry-command`),{textContent:`❯ `+n,title:n});let i=r.querySelector(`.entry-state`),a=r.querySelector(`.entry-output`);E.append(r);let o=E.querySelectorAll(`.entry`);o.length>100&&o[0].remove(),R=!0,E.setAttribute(`aria-busy`,`true`),Y.setActive(!1),G(),K(!0),W(`命令执行中`);let s=!1,c=``;try{let e=``,r=``,i=0,o=`o`;if(n===`exit`)r=`请关闭模块 WebUI 页面退出终端`,i=1;else if(n===`help`||n.startsWith(`help `))e=ge(n.slice(4).trim()),o=`help`;else if(n.startsWith(`!`)){let t=n.slice(1).trim();t?{out:e,err:r,code:i}=await le(t):(r=`请在 ! 后输入 Shell 命令`,i=1)}else{let a=t(n),o=await se(a);({out:e,err:r,code:i}=o);let c=d(o);s=!c.ok,s&&!r&&(c.code.startsWith(`transport.`)||!e)&&(r=c.message),a.includes(`--raw`)||(e=fe(e)),[`service`,`sub`,`node`,`catalog`].includes(a[0])&&He()}s||=i!==0,Z(a,o,e),Z(a,`e`,r),c=[e,r].filter(Boolean).join(`
`)}catch(e){s=!0,c=`异常: ${e instanceof Error?e.message:String(e)}`,Z(a,`e`,c)}finally{H=c,A.textContent=`复制结果`,r.dataset.state=s?`error`:`success`,i.textContent=s?`失败`:`完成`,R=!1,E.setAttribute(`aria-busy`,`false`),G(),K(),W(s?`命令执行失败，请查看输出`:`命令执行完成`),Y.setActive(!document.hidden)}}function We(){if(R||z)return!1;let e=u(O.value,U,Pe,Fe);if(!e.candidates.length)return q(),!1;if(Ve(e.completed),e.candidates.length>1){for(let t of e.candidates){let e=Object.assign(document.createElement(`button`),{type:`button`,textContent:t});e.dataset.completion=t,k.append(e)}k.hidden=!1,W(`${e.candidates.length} 个补全候选，向下键选择`)}return!0}function Q(e){R||!F.length||e===1&&I===-1||(I===-1&&(L=O.value,I=F.length),I=Math.max(0,I+e),I>=F.length?(I=-1,O.value=L):O.value=F[I],q(),G(),J())}D.addEventListener(`submit`,e=>{e.preventDefault(),Ue(O.value)}),O.addEventListener(`compositionstart`,()=>{z=!0}),O.addEventListener(`compositionend`,()=>{z=!1,G()}),O.addEventListener(`input`,()=>{I=-1,q(),G()}),O.addEventListener(`keydown`,e=>{e.isComposing||z||e.keyCode===229||(e.key===`Tab`&&!e.shiftKey&&We()?e.preventDefault():e.key===`ArrowDown`&&!k.hidden?(e.preventDefault(),k.querySelector(`button`)?.focus()):e.key===`ArrowUp`||e.key===`ArrowDown`?(e.preventDefault(),Q(e.key===`ArrowUp`?-1:1)):e.key===`Enter`?(e.preventDefault(),e.repeat||Ue(O.value)):e.key===`l`&&e.ctrlKey&&!R&&(e.preventDefault(),X()))}),k.addEventListener(`keydown`,e=>{if(e.key!==`ArrowDown`&&e.key!==`ArrowUp`)return;e.preventDefault();let t=Array.from(k.querySelectorAll(`button`)),n=t.indexOf(document.activeElement)+(e.key===`ArrowDown`?1:-1);n<0?J():t[Math.min(n,t.length-1)]?.focus()}),document.addEventListener(`keydown`,e=>{e.key===`Escape`&&!k.hidden&&(q(),J())}),document.addEventListener(`pointerdown`,e=>{D.contains(e.target)||q()}),document.addEventListener(`click`,e=>{let t=e.target.closest(`[data-command], [data-completion]`);t&&!R&&Ve(t.dataset.command??te(O.value,t.dataset.completion))}),Ee.addEventListener(`click`,We),De.addEventListener(`click`,()=>Q(-1)),Oe.addEventListener(`click`,()=>Q(1)),j.addEventListener(`click`,X),A.addEventListener(`click`,async()=>{let e=H,t=!1;try{await navigator.clipboard.writeText(e),t=!0}catch{}e===H&&(A.textContent=t?`已复制`:`复制失败`,W(t?`最近一次命令结果已复制`:`复制失败，请选中输出手动复制`))}),E.addEventListener(`scroll`,()=>{B=E.scrollHeight-E.clientHeight-E.scrollTop<24,M.hidden=B},{passive:!0}),M.addEventListener(`click`,()=>K(!0));var $=new ResizeObserver(()=>K());$.observe(E),document.addEventListener(`visibilitychange`,()=>Y.setActive(!document.hidden&&!R)),window.addEventListener(`pagehide`,()=>{Y.setActive(!1),V&&cancelAnimationFrame(V),$.disconnect()}),window.addEventListener(`pageshow`,()=>{$.observe(E),Y.setActive(!document.hidden&&!R)}),T(`environment`).hidden=!0,He(),G(),Y.setActive(!document.hidden);