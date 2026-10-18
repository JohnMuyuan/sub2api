export default {
  title: '号池统计', description: '查看获授权号池内的账号状态与用量统计',
  groups: '可查看的账号分组', groupsHint: '仅允许查看账号状态、额度快照和统计，邮箱已隐藏。未选择分组时没有可见账号；此处不额外授予 API 调用权限。',
  refresh: '刷新', loading: '加载中…', error: '加载失败，请稍后重试',
  empty: '暂无可见账号，请联系管理员分配号池', account: '账号', platform: '平台', status: '状态', pool: '号池', quota: '额度快照',
  noSnapshot: '暂无快照', stale: '历史快照', resets: '重置时间', observed: '采样时间',
  active: '正常', disabled: '已禁用', accountError: '异常', paused: '暂停调度', limited: '暂时受限',
  stats: '查看统计', requests: '请求数', tokens: 'Token 数', cost: '账号费用（USD）', date: '日期',
  period: '最近 {days} 天', previous: '上一页', next: '下一页', total: '共 {count} 个账号',
  statsHint: '统计按系统时区汇总；额度来自已有记录，刷新页面不会发起上游额度查询。',
  noHistory: '该时间段内没有用量记录'
}
