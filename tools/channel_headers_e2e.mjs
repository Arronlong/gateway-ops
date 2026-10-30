import { createRequire } from "node:module"
import { fileURLToPath } from "node:url"
import { mkdirSync, writeFileSync, readdirSync } from "node:fs"
import path from "node:path"
import { spawnSync } from "node:child_process"

const root=fileURLToPath(new URL("../",import.meta.url)), dir=path.join(root,"outputs","channel-headers-browser")
const require=createRequire(path.join(root,"frontend/package.json"))
const {build}=createRequire(require.resolve("vite"))("esbuild")
mkdirSync(dir,{recursive:true})
const entry=`
import React from "react";
import {createRoot} from "react-dom/client";
import {ChannelFormDialog} from "../../frontend/components/monitor/channel-form-dialog";
const pause=()=>new Promise(r=>setTimeout(r,80));
const root=createRoot(document.getElementById("app"));
const cases=[
{name:"newapi_password",type:"newapi",credential_mode:"password"},
{name:"newapi_cookie",type:"newapi",credential_mode:"token",newapi_auth_type:"cookie"},
{name:"newapi_access_token",type:"newapi",credential_mode:"token",newapi_auth_type:"access_token"},
{name:"sub2api_password",type:"sub2api",credential_mode:"password"},
{name:"sub2api_token",type:"sub2api",credential_mode:"token"}
];
const results=[];
function input(selector,value){const el=typeof selector==="string"?document.querySelector(selector):selector;if(!el)throw Error("missing input "+selector);const proto=el instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;Object.getOwnPropertyDescriptor(proto,"value").set.call(el,value);el.dispatchEvent(new Event("input",{bubbles:true}));}
function button(text){const el=[...document.querySelectorAll("button")].find(e=>e.textContent.trim()===text);if(!el)throw Error("missing button "+text);el.click();}
async function prepare(tc,edit){
const channel={id:7,type:tc.type,name:tc.name,username:"monitor",site_url:"https://example.invalid",credential_mode:tc.credential_mode,newapi_auth_type:tc.newapi_auth_type,newapi_user_id:"7",balance_mode:"auto",balance_threshold:0,monitor_enabled:false,accounts:[{id:8,is_primary:false,username:"additional",credential_mode:tc.credential_mode,newapi_auth_type:tc.newapi_auth_type,newapi_user_id:"8",turnstile_enabled:false,sub2api_refresh_token_configured:true}],login_headers:[{key:"X-Custom-Header",value:"before"},...(tc.name==="sub2api_token"?[{key:"authorization",value:"Configured {{access_token}}"}]:[]),...(tc.name==="newapi_cookie"?[{key:"cookie",value:"session=saved"},{key:"new-api-user",value:"41"}]:[]),...(tc.name==="newapi_access_token"?[{key:"Cookie",value:"{{cookie}}"},{key:"New-Api-User",value:"{{user_id}}"}]:[])],sub2api_refresh_token_configured:true};
root.render(<ChannelFormDialog key={tc.name+edit} open channel={edit?channel:undefined} onOpenChange={()=>{}}/>);await pause();
if(!edit){
if(tc.type==="sub2api"){document.querySelector("#type").click();await pause();document.querySelector('[role="option"][data-state]')&&[...document.querySelectorAll('[role="option"]')].find(e=>e.textContent.trim()==="Sub2API").click();await pause();}
const group=document.querySelector('[role="group"][aria-label="登录方式"]');
if(!group)throw Error("missing single-level login-method selector");
if(group.querySelectorAll("button").length!==(tc.type==="newapi"?3:2))throw Error("wrong login-method option count");
if(tc.credential_mode==="token"){const label=tc.type==="sub2api"?"Token":tc.newapi_auth_type==="cookie"?"Cookie + User ID":"访问令牌";const choice=[...group.querySelectorAll("button")].find(e=>e.textContent.trim()===label);if(!choice)throw Error("missing direct login method "+label);choice.click();await pause();}
input("#name",tc.name);await pause();input("#site_url","https://example.invalid");await pause();
if(tc.credential_mode==="password"){input("#gatewayops-channel-username","monitor");await pause();input("#gatewayops-channel-password","password-test");await pause();}
else if(tc.type==="sub2api"){input("#sub2api-token","access-test");await pause();input("#sub2api-refresh-token","refresh-test");await pause();}
else if(tc.newapi_auth_type==="cookie"){input("#newapi-cookie","session=test");await pause();input("#newapi-user-id","7");await pause();}
else {input("#newapi-access-token-user-id","7");await pause();input("#newapi-access-token","access-test");await pause();}
}
const methodGroup=document.querySelector('[role="group"][aria-label="登录方式"]');
if(!methodGroup)throw Error("missing single-level login-method selector");
if(document.querySelector('[role="dialog"]').textContent.includes("NewAPI Token 方式"))throw Error("duplicate nested login-method selector");
const activeMethod=[...methodGroup.querySelectorAll("button")].find(e=>e.getAttribute("aria-pressed")==="true");
const expectedMethod=tc.credential_mode==="password"?"账号密码":tc.type==="sub2api"?"Token":tc.newapi_auth_type==="cookie"?"Cookie + User ID":"访问令牌";
if(activeMethod?.textContent.trim()!==expectedMethod)throw Error("wrong selected login method");
const methodButtons=[...methodGroup.querySelectorAll("button")];
if(methodButtons.length!==(tc.type==="newapi"?3:2))throw Error("wrong method option count");
for(const btn of methodButtons){const r=btn.getBoundingClientRect();if(r.left<0||r.right>innerWidth||btn.scrollWidth>btn.clientWidth)throw Error("method option clipped");}
if(tc.type==="newapi"&&tc.credential_mode==="token"){
const selectedText=activeMethod.textContent.trim();
for(const label of ["Cookie + User ID","访问令牌",selectedText]){[...methodGroup.querySelectorAll("button")].find(e=>e.textContent.trim()===label).click();await pause();}
const selector=tc.credential_mode==="password"?"#gatewayops-channel-password":tc.newapi_auth_type==="cookie"?"#newapi-cookie":"#newapi-access-token";
if(!edit&&!document.querySelector(selector)?.value)throw Error("switching login methods lost input");
}
if(!edit){button("添加账号");await pause();input("#additional-account-0","additional");await pause();}
const accountGroup=document.querySelector('[role="group"][aria-label="账号 2 登录方式"]');
if(!accountGroup||accountGroup.querySelectorAll("button").length!==(tc.type==="newapi"?3:2))throw Error("additional account needs one login-method selector");
if(!edit){[...accountGroup.querySelectorAll("button")].find(e=>e.textContent.trim()===expectedMethod).click();await pause();
if(tc.credential_mode==="password"){input("#additional-password-0","password-test");await pause();}
else if(tc.type==="sub2api"){input("#additional-token-0","access-test");await pause();input("#additional-refresh-token-0","refresh-test");await pause();}
else if(tc.newapi_auth_type==="cookie"){input("#additional-cookie-0","session=test");await pause();input("#additional-user-id-0","8");await pause();}
else{input("#additional-newapi-access-token-0","access-test");await pause();input("#additional-access-token-user-id-0","8");await pause();}}
if(accountGroup.querySelector('[aria-pressed="true"]')?.textContent.trim()!==expectedMethod)throw Error("wrong additional account login method");
const panel=[...document.querySelectorAll("p")].find(el=>el.textContent==="自定义 Headers")?.closest(".rounded-lg");
if(document.querySelector('[data-sub2api-authorization]'))throw Error("Authorization must share the ordinary header list");
if(tc.type==="sub2api"&&tc.credential_mode==="token"){
const authKey=[...panel.querySelectorAll('input[aria-label^="Headers Key"]')].find(e=>e.value.toLowerCase()==="authorization");
if(!authKey)throw Error("missing editable Authorization in shared header list");
const row=authKey.parentElement,authValue=row.querySelector('input[aria-label^="Headers Value"]');
if(authKey.readOnly||authValue.readOnly||!row.querySelector("button"))throw Error("Authorization must be editable like every other header");
if(authValue.value!==(edit?"Configured {{access_token}}":"Bearer {{access_token}}"))throw Error("default or saved Authorization template was replaced");
input(authKey,"authorization");await pause();input(authValue,"Token {{access_token}}");await pause();
}
if(tc.name==="newapi_cookie"&&!edit){
for(const [name,original,replacement] of [["cookie","{{cookie}}","session=override"],["new-api-user","{{user_id}}","42"]]){
const key=[...panel.querySelectorAll('input[aria-label^="Headers Key"]')].find(e=>e.value.toLowerCase()===name);
if(!key)throw Error("missing editable "+name+" in the shared header list");
const row=key.parentElement,value=row.querySelector('input[aria-label^="Headers Value"]');
if(key.readOnly||value.readOnly||!row.querySelector("button"))throw Error(name+" must be editable like every other header");
if(value.value!==original)throw Error("default or saved "+name+" was replaced");
input(key,name);await pause();input(value,replacement);await pause();
}
}
if(tc.type==="newapi"&&tc.credential_mode==="token"){
const names=[...panel.querySelectorAll('input[aria-label^="Headers Key"]')].map(e=>e.value.toLowerCase());
if([...document.querySelectorAll('p')].some(e=>e.textContent.trim()==="访问令牌 Headers"))throw Error("NewAPI token headers must share the ordinary header list");
if(tc.newapi_auth_type==="access_token"){
if(!names.includes("authorization")||!names.includes("new-api-user")||names.includes("cookie"))throw Error("access-token headers are not unified");
}else if(!names.includes("cookie")||!names.includes("new-api-user")||names.includes("authorization"))throw Error("cookie headers are not unified");
}
const key=panel?.querySelector('input[aria-label="Headers Key 1"]');if(!key)throw Error("missing common header editor");
const fixedSelector=tc.credential_mode==="password"?"#gatewayops-channel-password":tc.type==="sub2api"?"#sub2api-refresh-token":tc.newapi_auth_type==="access_token"?"#newapi-access-token":"#newapi-user-id";
const fixed=document.querySelector(fixedSelector);if(!fixed || !(fixed.compareDocumentPosition(key)&Node.DOCUMENT_POSITION_FOLLOWING))throw Error("custom headers must follow fixed credentials: "+fixedSelector);
panel.scrollIntoView({block:"center"});
const fixedRect=fixed.getBoundingClientRect(), panelRect=panel.getBoundingClientRect();if(fixedRect.bottom>panelRect.top)throw Error("custom headers overlap fixed credentials");
const rect=key.getBoundingClientRect();if(rect.width<=0||rect.left<0||rect.right>innerWidth)throw Error("header editor outside viewport");
input(key,"X-Custom-Header");await pause();input(panel.querySelector('input[aria-label="Headers Value 1"]'),"after");await pause();
window.__calls=[];document.querySelector("form").requestSubmit();await pause();
const call=window.__calls.find(c=>c.path.startsWith("/channels")&&c.options?.body);if(!call)throw Error("no saved request: "+document.querySelector('[role="alert"]')?.textContent+" "+document.body.innerText.slice(-350));
const body=JSON.parse(call.options.body);if(body.login_headers[0]?.key!=="X-Custom-Header"||body.login_headers[0]?.value!=="after")throw Error("headers not submitted");
if(tc.newapi_auth_type==="access_token"){
const credential=JSON.parse(body.token_credential);
if(!credential.headers?.some(item=>item.key==="Authorization")||!credential.headers?.some(item=>item.key==="New-Api-User"))throw Error("access-token headers lost");
if(credential.headers.some(item=>item.key.toLowerCase()==="cookie"))throw Error("cookie header leaked into access-token credential");
}
if(tc.type==="sub2api"&&tc.credential_mode==="token"){
if(!body.login_headers.some(item=>item.key==="authorization"&&item.value==="Token {{access_token}}"))throw Error("edited Authorization was not saved");
for(const raw of [body.token_credential,body.accounts[0].token_credential])if(raw){const credential=JSON.parse(raw);if(credential.access_token!=="access-test"||credential.refresh_token!=="refresh-test")throw Error("automatic Authorization changed saved tokens");}
}
if(tc.name==="newapi_cookie"&&!edit)for(const [key,value] of [["cookie","session=override"],["new-api-user","42"]])if(!body.login_headers.some(item=>item.key===key&&item.value===value))throw Error("edited "+key+" was not saved");
if(body.credential_mode!==tc.credential_mode||body.accounts?.[0]?.credential_mode!==tc.credential_mode)throw Error("login method not submitted");
if(tc.type==="newapi"&&tc.credential_mode==="token")for(const credential of [body.token_credential,body.accounts[0].token_credential]){if((!edit||tc.newapi_auth_type==="access_token")&&JSON.parse(credential).auth_type!==tc.newapi_auth_type)throw Error("wrong serialized NewAPI login method");}
return {mode:tc.name,operation:edit?"edit":"create",ok:true};
}
(async()=>{for(const edit of [false,true])for(const tc of cases.filter(tc=>!window.__case||tc.name===window.__case)){try{results.push(await prepare(tc,edit));}catch(e){results.push({mode:tc.name,operation:edit?"edit":"create",ok:false,error:String(e)});}}document.getElementById("result").textContent=JSON.stringify(results);document.body.dataset.done="true";})();
`
writeFileSync(path.join(dir,"entry.tsx"),entry,"utf8")
await build({entryPoints:[path.join(dir,"entry.tsx")],outfile:path.join(dir,"bundle.js"),bundle:true,format:"iife",jsx:"automatic",alias:{react:path.join(root,"frontend/node_modules/react"),"react-dom":path.join(root,"frontend/node_modules/react-dom")},tsconfig:path.join(root,"frontend","tsconfig.json"),define:{"process.env.NODE_ENV":'"test"'},plugins:[{name:"local-test-stubs",setup(b){b.onResolve({filter:/^@\/lib\/(queries|api)$/},args=>({path:args.path,namespace:"stub"}));b.onLoad({filter:/.*/,namespace:"stub"},args=>({contents:args.path.endsWith("queries")?'export const useCaptchaConfigs=()=>({data:[],loading:false,error:null});':'export async function apiFetch(path,options){window.__calls??=[];window.__calls.push({path,options});return {id:7};}'}));}}]})
const css=readdirSync(path.join(root,"frontend/dist/assets")).find(name=>name.startsWith("index-")&&name.endsWith(".css"))
writeFileSync(path.join(dir,"index.html"),'<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="../../frontend/dist/assets/'+css+'"><div id="app"></div><pre id="result"></pre><script>window.__case='+JSON.stringify(process.env.HEADER_TEST_CASE||"")+';</script><script src="bundle.js"></script>',"utf8")
const chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
const res=spawnSync(chrome,["--headless=new","--disable-gpu","--no-first-run","--no-default-browser-check","--allow-file-access-from-files","--window-size="+(process.env.HEADER_TEST_WIDTH||"1366")+",900","--screenshot="+path.join(dir,"headers-"+(process.env.HEADER_TEST_WIDTH||"1366")+".png"),"--user-data-dir="+path.join(dir,"profile"),"--virtual-time-budget=30000","--dump-dom",new URL("file:///"+path.join(dir,"index.html").replaceAll("\\","/")).href],{encoding:"utf8",timeout:60000,windowsHide:true,maxBuffer:4*1024*1024})
writeFileSync(path.join(dir,"browser.html"),res.stdout??"","utf8")
const match=res.stdout?.match(/<pre id="result"[^>]*>([\s\S]*?)<\/pre>/)
if(!match?.[1]){console.error("browser did not return test results",res.status,res.error?.message,res.stderr?.slice(-2000));process.exit(1)}
const results=JSON.parse(match[1].replaceAll("&amp;","&").replaceAll("&lt;","<").replaceAll("&gt;",">"))
for(const r of results)console.log(JSON.stringify(r))
if(results.length!==(process.env.HEADER_TEST_CASE?2:10)||results.some(r=>!r.ok))process.exit(1)
