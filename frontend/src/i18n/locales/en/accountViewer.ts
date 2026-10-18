export default {
  title: 'Pool statistics', description: 'Account status and usage in your assigned pools',
  groups: 'Visible account groups', groupsHint: 'View status, quota snapshots and statistics only. Email addresses are hidden. No groups means no visible accounts. This does not grant additional API access.',
  refresh: 'Refresh', loading: 'Loading…', error: 'Unable to load. Please try again.',
  empty: 'No visible accounts. Ask an administrator to assign pools.', account: 'Account', platform: 'Platform', status: 'Status', pool: 'Pool', quota: 'Quota snapshot',
  noSnapshot: 'No snapshot', stale: 'Historical snapshot', resets: 'Resets', observed: 'Observed',
  active: 'Active', disabled: 'Disabled', accountError: 'Error', paused: 'Scheduling paused', limited: 'Temporarily limited',
  stats: 'View statistics', requests: 'Requests', tokens: 'Tokens', cost: 'Account cost (USD)', date: 'Date',
  period: 'Last {days} days', previous: 'Previous', next: 'Next', total: '{count} accounts',
  statsHint: 'Statistics use the system timezone. Quotas use stored snapshots; refreshing does not query upstream services.',
  noHistory: 'No usage in this period'
}
