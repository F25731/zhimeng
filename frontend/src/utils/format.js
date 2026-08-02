export const statusLabels = {
  unused: "未使用",
  reserved: "填写中",
  provisioning: "创建中",
  active: "运行中",
  warning: "告警",
  offline: "离线",
  pending: "待执行",
  running: "执行中",
  completed: "已完成",
  failed: "失败",
  manual_intervention: "需人工处理",
  revoked: "已撤销",
  expired: "已过期",
  stopped: "已停止",
  frozen: "已冻结",
  upgrading: "升级中",
  deleting: "删除中",
  deleted: "已删除",
  published: "已发布",
  draft: "草稿",
  online: "在线"
};

export const statusTypes = {
  active: "success",
  online: "success",
  completed: "success",
  published: "success",
  warning: "warning",
  reserved: "warning",
  provisioning: "warning",
  running: "warning",
  upgrading: "warning",
  deleting: "danger",
  deleted: "info",
  failed: "danger",
  offline: "danger",
  revoked: "danger",
  manual_intervention: "danger",
  stopped: "info",
  frozen: "info",
  expired: "info",
  unused: "info",
  pending: "info",
  draft: "info"
};

export function formatDate(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false
  }).format(date);
}

export function compactNumber(value) {
  return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(Number(value || 0));
}
