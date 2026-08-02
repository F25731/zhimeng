<template>
  <div class="public-shell">
    <header class="public-header">
      <div class="public-brand"><img class="brand-logo" src="/logo.svg" alt="" /><span>分站开通</span></div>
      <div class="public-header__meta">任务 {{ shortJobId }}</div>
    </header>
    <main class="public-main">
      <section class="provision-panel progress-panel">
        <aside class="provision-sidebar">
          <h1>{{ title }}</h1>
          <p>{{ subtitle }}</p>
          <el-progress type="circle" :percentage="Number(job.progress || 0)" :status="progressStatus" :width="132" :stroke-width="8" />
        </aside>
        <div class="provision-body">
          <div class="provision-body__head">
            <h2>{{ currentMessage }}</h2>
            <p>开站过程已持久化，可以关闭页面后使用同一卡密继续查看。</p>
          </div>
          <div class="progress-summary">
            <el-progress :percentage="Number(job.progress || 0)" :status="progressStatus" :stroke-width="8" />
            <p class="field-hint">当前步骤：{{ stepLabel(job.currentStep || job.current_step) }}</p>
          </div>

          <el-alert v-if="job.status === 'failed'" type="error" :title="job.errorMessage || job.error_message || '部署任务执行失败'" :closable="false" show-icon />
          <el-alert v-if="disconnected && !terminal" type="warning" title="实时连接已断开，系统正在自动轮询任务状态。" :closable="false" show-icon />

          <div class="progress-events" aria-live="polite">
            <div v-for="event in events" :key="event.id" class="progress-event">
              <i :class="event.status === 'failed' ? 'el-icon-error danger' : 'el-icon-success'" />
              <div><strong>{{ event.message }}</strong><div class="field-hint">{{ stepLabel(event.step) }}</div></div>
              <time>{{ formatDate(event.createdAt) }}</time>
            </div>
            <el-empty v-if="!events.length" class="empty-compact" description="等待任务事件" :image-size="64" />
          </div>

          <div v-if="completed" class="completion-actions">
            <el-button type="primary" icon="el-icon-top-right" @click="openSite">进入分站</el-button>
            <el-button icon="el-icon-document-copy" @click="copy(siteUrl)">复制地址</el-button>
          </div>
          <div v-else-if="job.status === 'failed'" class="completion-actions">
            <el-button type="primary" icon="el-icon-refresh" :loading="retrying" @click="retry">重试创建</el-button>
            <el-button @click="$router.push('/')">返回开站页</el-button>
          </div>
        </div>
      </section>
    </main>
    <footer class="public-footer">任务状态由数据库持久化保存</footer>
  </div>
</template>

<script>
import { getJob, retryPublicJob } from "@/api/control";
import { formatDate } from "@/utils/format";

const stepNames = {
  pending: "等待执行", validating: "校验参数", allocating_node: "分配部署节点",
  generating_secrets: "生成独立密钥", creating_database: "创建独立数据库",
  generating_config: "生成站点配置", pulling_image: "准备应用镜像",
  starting_containers: "启动应用和任务服务", initializing_database: "初始化数据结构",
  creating_admin: "创建站点管理员", applying_branding: "应用网站品牌",
  checking_health: "检查应用就绪状态", waiting_worker: "等待 Worker 心跳",
  activating_route: "开放域名路由", checking_route: "确认域名路由",
  checking_https: "检查公网 HTTPS", active: "站点已激活", failed: "任务失败"
};

export default {
  name: "ProgressPage",
  data() {
    return {
      job: {}, events: [], eventSource: null, retrying: false, disconnected: false,
      pollTimer: null, token: sessionStorage.getItem("provision_token") || ""
    };
  },
  computed: {
    completed() { return this.job.status === "completed"; },
    terminal() { return this.completed || this.job.status === "failed"; },
    progressStatus() { return this.completed ? "success" : this.job.status === "failed" ? "exception" : undefined; },
    title() { return this.completed ? "分站创建完成" : this.job.status === "failed" ? "创建未完成" : "正在创建分站"; },
    subtitle() { return this.completed ? "站点服务已通过健康检查，可以开始使用。" : "请稍候，系统正在自动完成隔离部署。"; },
    shortJobId() { return String(this.$route.params.jobId || "").slice(0, 8); },
    currentMessage() {
      const latest = this.events[this.events.length - 1];
      return latest ? latest.message : this.completed ? "分站已创建" : "等待部署任务开始";
    },
    result() {
      try { return JSON.parse(this.job.resultJSON || this.job.result_json || "{}"); } catch (_) { return {}; }
    },
    siteUrl() { return this.result.siteUrl || ""; }
  },
  async created() {
    if (!this.token) { this.$router.replace("/"); return; }
    await this.load();
    if (!this.terminal) this.connect();
  },
  beforeDestroy() { this.closeConnections(); },
  methods: {
    formatDate,
    stepLabel(value) { return stepNames[value] || value || "等待执行"; },
    normalizeEvent(item) {
      return {
        id: String(item.sequence || item.id || ""),
        step: item.step,
        status: item.status,
        progress: item.progress,
        message: item.publicMessage || item.public_message || item.message || "",
        createdAt: item.createdAt || item.created_at
      };
    },
    mergeEvents(items) {
      (items || []).forEach((item) => {
        const event = this.normalizeEvent(item);
        if (event.id && !this.events.some((existing) => existing.id === event.id)) this.events.push(event);
      });
      this.events.sort((a, b) => Number(a.id) - Number(b.id));
    },
    async load(silent = false) {
      try {
        const result = await getJob(this.$route.params.jobId, this.token);
        this.job = result.job;
        this.mergeEvents(result.events);
        if (this.terminal) this.closeConnections();
      } catch (error) {
        if (!silent) this.$message.error(error.userMessage || "任务不存在或开站会话已过期");
      }
    },
    connect() {
      if (this.eventSource) this.eventSource.close();
      const url = `/api/public/provision/jobs/${this.$route.params.jobId}/events?token=${encodeURIComponent(this.token)}`;
      this.eventSource = new EventSource(url);
      ["progress", "completed", "failed"].forEach((type) => this.eventSource.addEventListener(type, (event) => {
        const data = JSON.parse(event.data);
        this.mergeEvents([{ id: event.lastEventId, ...data }]);
        this.job = { ...this.job, progress: data.progress, status: data.status, currentStep: data.step };
        this.disconnected = false;
        if (type !== "progress") { this.closeConnections(); this.load(); }
      }));
      this.eventSource.onopen = () => {
        this.disconnected = false;
        if (this.pollTimer) { clearInterval(this.pollTimer); this.pollTimer = null; }
      };
      this.eventSource.onerror = () => {
        this.disconnected = true;
        if (!this.pollTimer && !this.terminal) {
          this.load(true);
          this.pollTimer = setInterval(() => this.load(true), 3000);
        }
      };
    },
    closeConnections() {
      if (this.eventSource) { this.eventSource.close(); this.eventSource = null; }
      if (this.pollTimer) { clearInterval(this.pollTimer); this.pollTimer = null; }
    },
    async retry() {
      this.retrying = true;
      try {
        await retryPublicJob(this.$route.params.jobId, this.token);
        this.events = [];
        await this.load();
        this.connect();
      } catch (error) { this.$message.error(error.userMessage || "任务无法重试"); }
      finally { this.retrying = false; }
    },
    openSite() { if (this.siteUrl) window.open(this.siteUrl, "_blank", "noopener"); },
    async copy(value) {
      if (!value) return;
      await navigator.clipboard.writeText(value);
      this.$message.success("分站地址已复制");
    }
  }
};
</script>
