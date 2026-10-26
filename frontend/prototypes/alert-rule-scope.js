const definitions = {
  channel: {
    label: '渠道', count: 218, matched: 6, unit: '个渠道',
    metrics: [['balance', '渠道余额低'], ['rate', '倍率变化']],
    filters: [['评分', '大于等于', '86'], ['缓存命中率', '大于等于', '60%'], ['渠道名称', '不包含', '测试 / 备用'], ['监控状态', '等于', '开启'], ['关联模型', '等于', 'gpt-4.1']],
    rows: [['主力 OpenAI', '评分 92 · 缓存率 74%', '监控开启'], ['Claude 官方', '评分 88 · 缓存率 66%', '监控开启']]
  },
  account: {
    label: '中转站账号', count: 164, matched: 23, unit: '个账号',
    metrics: [['spend', '账号消耗异常'], ['stable', '账号稳定性异常']],
    filters: [['站点类型', '等于', 'Sub2API'], ['账号状态', '等于', '启用'], ['调度状态', '等于', '允许调度'], ['账号名称', '包含', 'prod']],
    rows: [['prod-team-01', 'Sub2API · 调度中', '运行正常'], ['prod-team-03', 'NewAPI · 调度中', '运行正常']]
  },
  user: {
    label: '中转站用户', count: 3240, matched: 41, unit: '个用户',
    metrics: [['userSpend', '用户消耗异常'], ['multiIp', '用户多 IP'], ['userBalance', '用户余额低']],
    filters: [['站点类型', '等于', 'NewAPI'], ['用户状态', '等于', '启用'], ['风险等级', '大于等于', '中'], ['注册 IP 数', '大于等于', '2']],
    rows: [['user-1048', 'NewAPI · 风险中', '注册 IP 3 个'], ['user-2381', 'Sub2API · 风险中', '注册 IP 2 个']]
  }
};
const metricNames = Object.fromEntries(Object.values(definitions).flatMap(item => item.metrics));
const state = { type: 'channel', metric: 'balance', step: 0, logic: '全部', filters: null };
const content = document.getElementById('content');

function render() {
  document.querySelectorAll('.steps button').forEach(button => button.classList.toggle('active', Number(button.dataset.step) === state.step));
  document.getElementById('back').textContent = state.step ? '上一步' : '取消';
  document.getElementById('next').textContent = state.step === 3 ? '保存规则' : '下一步';
  [renderScope, renderCondition, renderAction, renderPolicy][state.step]();
}

function renderScope() {
  const item = definitions[state.type];
  if (!state.filters) state.filters = item.filters.map(row => row.slice());
  const types = Object.entries(definitions).map(([key, value]) => '<button class="' + (state.type === key ? 'active' : '') + '" data-type="' + key + '">' + value.label + '</button>').join('');
  const metrics = item.metrics.map(([key, label]) => '<button class="' + (state.metric === key ? 'active' : '') + '" data-metric="' + key + '">' + label + '</button>').join('');
  const filters = state.filters.map((row, index) => '<div class="filter"><select data-field="' + index + '"><option>' + row[0] + '</option>' + item.filters.filter(value => value[0] !== row[0]).map(value => '<option>' + value[0] + '</option>').join('') + '</select><select><option>' + row[1] + '</option><option>等于</option><option>不等于</option><option>包含</option><option>不包含</option><option>大于</option><option>大于等于</option><option>小于</option><option>小于等于</option></select><input value="' + row[2] + '"><button class="remove" data-remove="' + index + '">×</button></div>').join('');
  const preview = item.rows.map(row => '<tr><td>' + row[0] + '</td><td>' + row[1] + '</td><td>' + row[2] + '</td></tr>').join('');
  const rangeLabel = state.type === 'channel' ? '渠道范围' : '中转站范围';
  const rangeOptions = state.type === 'channel' ? '<option>全部渠道</option><option>指定渠道组</option><option>指定渠道</option>' : '<option>全部中转站（Sub2API + NewAPI）</option><option>仅 Sub2API</option><option>仅 NewAPI</option>';
  content.innerHTML = '<div class="fields"><div class="field"><label>规则名称</label><input class="input" value="渠道余额偏低时通知"></div><div class="field"><label>' + rangeLabel + '</label><select class="select">' + rangeOptions + '</select></div></div><div class="label" style="margin-top:15px">监控对象类型</div><div class="types">' + types + '</div><div class="label">预警指标</div><div class="metric-row">' + metrics + '</div><div class="bar"><strong>对象范围筛选</strong><span>每轮评估动态匹配，不固定为创建时的对象清单</span></div><section class="scope"><div class="scope-head"><div class="logic"><button data-logic="全部" class="' + (state.logic === '全部' ? 'active' : '') + '">满足全部</button><button data-logic="任一" class="' + (state.logic === '任一' ? 'active' : '') + '">满足任一</button></div><span class="help">字段随对象类型变化</span></div>' + filters + '<button class="add" id="add">＋ 添加筛选条件</button><div class="scope-foot"><span><strong>' + item.matched + ' ' + item.unit + '</strong>　符合筛选 · 总计 ' + item.count + ' ' + item.unit + '</span><span>试算示例 · 保存前重新计算</span></div></section><section class="preview"><h3>匹配对象预览 · 前 2 条</h3><table><thead><tr><th>' + item.label + '名称</th><th>关键属性</th><th>状态</th></tr></thead><tbody>' + preview + '</tbody></table></section><div class="note">渠道余额仅评估监控已开启且有有效余额快照的渠道；停止监控或采集失败标记为“数据未知”，不会按余额为 0 处理。健康评分、缓存率样本不足时不参与筛选。</div>';
  content.querySelectorAll('[data-type]').forEach(button => button.onclick = () => { state.type = button.dataset.type; state.filters = null; state.metric = definitions[state.type].metrics[0][0]; render(); });
  content.querySelectorAll('[data-metric]').forEach(button => button.onclick = () => { state.metric = button.dataset.metric; render(); });
  content.querySelectorAll('[data-logic]').forEach(button => button.onclick = () => { state.logic = button.dataset.logic; render(); });
  content.querySelectorAll('[data-remove]').forEach(button => button.onclick = () => { state.filters.splice(Number(button.dataset.remove), 1); render(); });
  content.querySelectorAll('[data-field]').forEach(select => select.onchange = () => { const row = item.filters.find(value => value[0] === select.value); if (row) state.filters[Number(select.dataset.field)] = row.slice(); render(); });
  document.getElementById('add').onclick = () => { const row = item.filters.find(value => !state.filters.some(current => current[0] === value[0])); if (row) { state.filters.push(row.slice()); render(); } };
}

function renderCondition() {
  const metric = state.metric;
  const spend = metric === 'spend' || metric === 'userSpend';
  const balance = metric === 'balance' || metric === 'userBalance';
  const multiIp = metric === 'multiIp';
  const threshold = balance ? '20' : multiIp ? '3' : spend ? '2' : '10';
  const unit = balance ? 'USD' : multiIp ? '个不同 IP / 1 小时' : spend ? '倍历史基线' : '% 错误率';
  content.innerHTML = '<div class="bar"><strong>' + metricNames[metric] + ' · 判定逻辑</strong><span>判定条件与对象筛选分开配置</span></div><div class="grid3"><section class="box"><h3>评估周期</h3><input class="input" value="15"> 分钟评估一次<p>使用滑动窗口；余额快照过期或采集失败时标记未知。</p></section><section class="box"><h3>异常阈值</h3><input class="input" value="' + threshold + '"> ' + unit + '<p>阈值针对每个匹配对象分别计算。</p></section><section class="box"><h3>触发去抖</h3><input class="input" value="3"> 次命中 / 最近 5 次评估<p>连续命中或 N 次中 M 次。</p></section></div><div class="grid3" style="margin-top:10px"><section class="box"><h3>最小样本</h3><input class="input" value="20"> 次请求<p>样本不足时不判异常。</p></section><section class="box"><h3>恢复滞回</h3><input class="input" value="1.5"> 倍触发值，连续 3 次</section><section class="box"><h3>数据有效性</h3><p>数据过期或缺失按未知处理，不当作正常或零。</p></section></div><div class="info">' + (spend ? '消耗异常结合历史基线、最小请求数、N 次中 M 次和恢复滞回，降低低流量与单笔大额误报。' : multiIp ? '多 IP 按滑动 1 小时内的不同来源 IP 数计算，可配置可信 IP 白名单和最小请求数。' : '恢复阈值留出滞回，避免事件反复开合。') + '</div>';
}

function notifications() {
  return '<section class="box"><h3>消息通知</h3><p>内置模板包含对象、指标、阈值、实际值、触发时间与处理结果。</p><p><label><input type="checkbox" checked> 飞书告警群 · 已启用</label></p><p><label><input type="checkbox" checked> 值班邮件 · 已启用</label></p><p><label><input type="checkbox"> Webhook-备用 · 已停用</label></p></section>';
}

function renderAction() {
  if (state.type === 'channel') {
    content.innerHTML = '<section class="box"><h3>渠道规则处理</h3><p>渠道对象当前产生预警事件并发送通知；不自动修改渠道启停状态。</p></section>' + notifications();
    return;
  }
  const user = state.type === 'user';
  const disable = user ? '自动停用用户' : '自动停止账号调度';
  const enable = user ? '自动启用用户' : '自动恢复账号调度';
  content.innerHTML = '<div class="fields"><section class="box"><h3>命中后的自动化处理</h3><select class="select"><option>不自动处理，仅记录事件</option><option>' + disable + '</option><option>' + enable + '</option></select><p><label><input type="checkbox"> 恢复后自动恢复本规则此前停用的状态</label></p><p>状态被人工修改或受其他规则约束时不自动恢复。</p></section>' + notifications() + '</div><div class="info">站点不支持的动作标记为不可用；Sub2API 与 NewAPI 分别校验能力，不假设行为一致。</div>';
}

function renderPolicy() {
  content.innerHTML = '<div class="label">预警级别</div><div class="severity"><label><input type="radio" name="level"> 提示</label><label><input type="radio" name="level" checked> 警告</label><label><input type="radio" name="level"> 严重</label></div><div class="fields" style="margin-top:14px"><div class="field"><label>异常通知冷却</label><input class="input" value="30 分钟 / 同一对象与事件"></div><div class="field"><label>保存状态</label><select class="select"><option>保存后立即启用</option><option>保存为暂停状态</option></select></div></div><dl class="summary"><dt>监控范围</dt><dd>' + definitions[state.type].label + ' · ' + definitions[state.type].matched + ' ' + definitions[state.type].unit + ' · 满足' + state.logic + '筛选</dd><dt>预警指标</dt><dd>' + metricNames[state.metric] + '</dd><dt>告警级别</dt><dd>警告</dd><dt>通知策略</dt><dd>2 个渠道 · 冷却 30 分钟</dd></dl><div class="info">预警级别描述事件严重程度，可用于通知路由和列表筛选；它不属于对象范围条件。</div>';
}

document.querySelectorAll('.steps button').forEach(button => button.onclick = () => { state.step = Number(button.dataset.step); render(); });
document.getElementById('next').onclick = () => { if (state.step < 3) { state.step++; render(); } else alert('原型演示：没有向服务端保存规则。'); };
document.getElementById('back').onclick = () => { if (state.step) { state.step--; render(); } else document.querySelector('.shade').classList.add('hidden'); };
document.getElementById('close').onclick = () => document.querySelector('.shade').classList.add('hidden');
render();
