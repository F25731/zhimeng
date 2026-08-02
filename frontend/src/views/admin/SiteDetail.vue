<template>
  <div v-loading="loading">
    <div class="page-header">
      <div>
        <el-button type="text" icon="el-icon-arrow-left" @click="$router.push('/zhimeng/sites')">返回分站列表</el-button>
        <h1>{{ site.name || '分站详情' }}</h1>
        <p class="mono">{{ site.domain || '-' }}</p>
      </div>
      <div class="page-header__actions">
        <StatusTag :status="site.status" />
        <el-button icon="el-icon-top-right" @click="openSite">访问分站</el-button>
        <el-dropdown trigger="click" @command="operate">
          <el-button>站点操作<i class="el-icon-arrow-down el-icon--right" /></el-button>
          <el-dropdown-menu slot="dropdown">
            <el-dropdown-item v-for="action in availableActions" :key="action.value" :command="action.value" :icon="action.icon" :divided="action.divided" :style="action.danger ? { color: '#d83a3a' } : null">{{ action.label }}</el-dropdown-item>
          </el-dropdown-menu>
        </el-dropdown>
        <el-button type="primary" icon="el-icon-upload2" :disabled="!['active','warning'].includes(site.status)" @click="openUpgrade">升级版本</el-button>
      </div>
    </div>

    <section class="data-panel">
      <div class="panel-body">
        <el-tabs v-model="activeTab" @tab-click="tabChanged">
          <el-tab-pane label="基本信息" name="overview">
            <div class="tab-content">
              <div class="detail-grid">
                <div v-for="item in details" :key="item.label" class="detail-item"><div class="detail-item__label">{{ item.label }}</div><div :class="['detail-item__value', item.mono && 'mono']">{{ item.value || '-' }}</div></div>
              </div>
              <el-alert v-if="site.last_error_message" class="site-error" type="error" :title="site.last_error_message" :description="site.last_error_code" :closable="false" show-icon />
              <el-alert v-if="site.route_error" class="site-error" type="warning" title="域名路由异常" :description="site.route_error" :closable="false" show-icon />
            </div>
          </el-tab-pane>
          <el-tab-pane label="运营数据" name="metrics">
            <div class="tab-content">
              <section class="stat-grid">
                <div v-for="item in operational" :key="item.label" class="stat-item"><div class="stat-item__label">{{ item.label }}</div><div class="stat-item__value">{{ item.value }}</div><div class="stat-item__meta">{{ item.meta }}</div></div>
              </section>
              <div ref="metricsChart" class="chart metrics-chart" />
            </div>
          </el-tab-pane>
          <el-tab-pane label="上游配置" name="channels">
            <div class="tab-content">
              <div class="channel-toolbar">
                <div>
                  <strong>{{ channelItems.length ? `已上报 ${channelItems.length} 个渠道` : channelEmptyText }}</strong>
                  <span v-if="channels.received_at">最近上报 {{ formatDate(channels.received_at) }}</span>
                </div>
                <el-button size="small" icon="el-icon-refresh" :loading="channelsLoading" @click="loadChannels()">刷新快照</el-button>
              </div>
              <el-table :data="channelItems" :empty-text="channelEmptyText">
                <el-table-column prop="name" label="渠道名称" min-width="140" />
                <el-table-column prop="apiFormat" label="协议" width="100" />
                <el-table-column prop="baseUrl" label="接口地址" min-width="220" show-overflow-tooltip />
                <el-table-column label="模型数" width="90" align="right"><template slot-scope="{ row }">{{ Array.isArray(row.models) ? row.models.length : 0 }}</template></el-table-column>
                <el-table-column label="启用" width="80"><template slot-scope="{ row }"><el-tag size="small" :type="row.enabled ? 'success' : 'info'" effect="plain">{{ row.enabled ? '是' : '否' }}</el-tag></template></el-table-column>
                <el-table-column label="健康" width="90"><template slot-scope="{ row }"><el-tag size="small" :type="row.healthy ? 'success' : 'danger'" effect="plain">{{ row.healthy ? '正常' : '异常' }}</el-tag></template></el-table-column>
              </el-table>
              <p class="field-hint">配置版本 {{ channelRevision }} · 页面每 30 秒自动刷新 · 不展示任何 API Key</p>
            </div>
          </el-tab-pane>
          <el-tab-pane label="运维记录" name="jobs"><div class="tab-content"><JobsTable :site-id="$route.params.id" /></div></el-tab-pane>
        </el-tabs>
      </div>
    </section>

    <el-dialog title="升级分站" :visible.sync="upgradeDialog" width="480px">
      <el-form label-position="top">
        <el-form-item label="当前版本"><el-input :value="site.current_version || '-'" disabled /></el-form-item>
        <el-form-item label="目标版本"><el-select v-model="targetVersion" style="width:100%" placeholder="选择已发布版本"><el-option v-for="item in availableVersions" :key="item.id" :label="`${item.version} · ${item.channel}`" :value="item.version" /></el-select></el-form-item>
      </el-form>
      <el-alert type="info" title="升级前会自动备份数据库；健康检查失败时回滚应用版本。" :closable="false" show-icon />
      <span slot="footer"><el-button @click="upgradeDialog=false">取消</el-button><el-button type="primary" :disabled="!targetVersion" @click="upgrade">创建升级任务</el-button></span>
    </el-dialog>
  </div>
</template>

<script>
import * as echarts from "echarts";
import { getSite, getSiteChannels, getSiteMetrics, listVersions, siteAction } from "@/api/control";
import StatusTag from "@/components/StatusTag.vue";
import JobsTable from "@/views/admin/components/JobsTable.vue";
import { compactNumber, formatDate } from "@/utils/format";

export default {
  name: "SiteDetailPage",
  components: { StatusTag, JobsTable },
  data() { return { site: {}, metrics: [], channels: {}, loading: false, activeTab: "overview", metricsLoaded: false, channelsLoaded: false, channelsLoading: false, channelTimer: null, chart: null, upgradeDialog: false, versions: [], targetVersion: "" }; },
  computed: {
    details() {
      return [
        { label: "站点 ID", value: this.site.id, mono: true }, { label: "客户备注", value: this.site.remark },
        { label: "完整域名", value: this.site.domain, mono: true }, { label: "部署节点", value: this.site.node_name || "未分配" },
        { label: "域名路由", value: this.routeLabel(this.site.route_status) }, { label: "路由最后验证", value: formatDate(this.site.route_verified_at) },
        { label: "当前版本", value: this.site.current_version, mono: true }, { label: "目标版本", value: this.site.desired_version, mono: true },
        { label: "独立数据库", value: this.site.database_name, mono: true }, { label: "数据库账号", value: this.site.database_user, mono: true },
        { label: "App 容器", value: this.site.app_container_name, mono: true }, { label: "Worker 容器", value: this.site.worker_container_name, mono: true },
        { label: "最后心跳", value: formatDate(this.site.last_heartbeat_at) }, { label: "创建时间", value: formatDate(this.site.created_at) }
      ];
    },
    operational() {
      return [
        { label: "用户总数", value: compactNumber(this.site.users_total), meta: "最新快照" },
        { label: "今日调用", value: compactNumber(this.site.calls_today), meta: "当日累计" },
        { label: "近 7 日调用", value: compactNumber(this.site.calls_7d), meta: "滚动 7 天" },
        { label: "历史总调用", value: compactNumber(this.site.calls_lifetime), meta: `成功率 ${Number(this.site.success_rate || 0).toFixed(1)}%` }
      ];
    },
    channelItems() {
      let value = this.channels.channels_json || [];
      if (typeof value === "string") { try { value = JSON.parse(value); } catch (_) { value = []; } }
      return Array.isArray(value) ? value.map((item) => ({ ...item, models: this.normalizeJSON(item.models, []) })) : [];
    },
    channelEmptyText() { return this.channels.received_at ? "该分站当前未配置上游渠道" : "等待分站首次上报脱敏渠道快照"; },
    channelRevision() { const value = this.channels.config_revision || ""; return !value || value.startsWith("0001-") ? "-" : value; },
    availableVersions() { return this.versions.filter((item) => item.status === "published" && item.version !== this.site.current_version); },
    availableActions() {
	  if (this.site.status === "deleting") return [];
      const actions = [];
      if (["stopped","offline"].includes(this.site.status)) actions.push({ value: "start", label: "启动站点", icon: "el-icon-video-play" });
      if (["active","warning"].includes(this.site.status)) actions.push({ value: "restart", label: "重启站点", icon: "el-icon-refresh" }, { value: "stop", label: "暂停运行", icon: "el-icon-video-pause" }, { value: "freeze", label: "冻结站点", icon: "el-icon-lock" });
      if (this.site.status === "frozen") actions.push({ value: "unfreeze", label: "解除冻结", icon: "el-icon-unlock" });
      actions.push({ value: "backup", label: "立即备份", icon: "el-icon-folder-add" });
	  actions.push({ value: "delete", label: "彻底删除", icon: "el-icon-delete", divided: true, danger: true });
      return actions;
    }
  },
  async mounted() { window.addEventListener("resize", this.resizeChart); window.addEventListener("control-theme-change", this.themeChanged); this.channelTimer = window.setInterval(() => { if (this.activeTab === "channels") this.loadChannels(true); }, 30000); await this.load(); },
  beforeDestroy() { window.removeEventListener("resize", this.resizeChart); window.removeEventListener("control-theme-change", this.themeChanged); window.clearInterval(this.channelTimer); if (this.chart) this.chart.dispose(); },
  methods: {
    formatDate,
    routeLabel(value) { return ({ disabled: "未开放", unverified: "待验证", activating: "正在开放", verifying_https: "检查 HTTPS", active: "已接入", failed: "异常" })[value] || value || "未检测"; },
    normalizeJSON(value, fallback) { if (typeof value !== "string") return value || fallback; try { return JSON.parse(value); } catch (_) { return fallback; } },
    async load() { this.loading = true; try { this.site = await getSite(this.$route.params.id); } catch (error) { this.$message.error(error.userMessage || "分站详情加载失败"); } finally { this.loading = false; } },
    openSite() { if (this.site.domain) window.open(`https://${this.site.domain}`, "_blank", "noopener"); },
    async tabChanged(tab) {
      if (tab.name === "metrics" && !this.metricsLoaded) { const data = await getSiteMetrics(this.site.id); this.metrics = data.items || []; this.metricsLoaded = true; this.$nextTick(this.renderChart); }
      if (tab.name === "channels" && !this.channelsLoaded) await this.loadChannels(true);
    },
    async loadChannels(silent = false) {
      if (this.channelsLoading || !this.site.id) return;
      this.channelsLoading = true;
      try { this.channels = await getSiteChannels(this.site.id); this.channelsLoaded = true; }
      catch (error) { this.channels = {}; if (!silent) this.$message.error(error.userMessage || "上游配置快照加载失败"); }
      finally { this.channelsLoading = false; }
    },
    renderChart() {
      if (!this.$refs.metricsChart) return;
      const dark = document.documentElement.dataset.theme === "dark";
      if (!this.chart) this.chart = echarts.init(this.$refs.metricsChart, dark ? "dark" : undefined);
      const rows = [...this.metrics].reverse();
      this.chart.setOption({ backgroundColor: "transparent", tooltip: { trigger: "axis" }, color: ["#28a66a", "#4c8fd5"], legend: { data: ["近 7 日调用", "用户总数"], bottom: 0 }, grid: { top: 18, left: 12, right: 12, bottom: 36, containLabel: true }, xAxis: { type: "category", data: rows.map((row) => formatDate(row.received_at).slice(5, 16)), axisTick: { show: false } }, yAxis: { type: "value", minInterval: 1, splitLine: { lineStyle: { color: dark ? "#272e2a" : "#edf1ee" } } }, series: [{ name: "近 7 日调用", type: "line", smooth: true, symbol: "none", data: rows.map((row) => Number(row.calls_7d || 0)) }, { name: "用户总数", type: "line", smooth: true, symbol: "none", data: rows.map((row) => Number(row.users_total || 0)) }] });
    },
    themeChanged() { if (this.chart) { this.chart.dispose(); this.chart = null; } this.$nextTick(this.renderChart); },
    resizeChart() { if (this.chart) this.chart.resize(); },
    async operate(action) {
	  if (action === "delete") {
		await this.deleteSite();
		return;
	  }
      const labels = { start: "启动站点", stop: "暂停运行", restart: "重启站点", freeze: "冻结站点", unfreeze: "解除冻结", backup: "备份站点" };
      const descriptions = {
        stop: `暂停后将停止分站“${this.site.name}”的 App、Worker 和 Reporter，并关闭域名访问；数据库、用户数据和站点文件都会保留，之后可以重新启动。`,
        freeze: `冻结后将停止分站“${this.site.name}”并关闭域名访问，数据会保留；必须执行“解除冻结”才能恢复。`,
        restart: `重启分站“${this.site.name}”的运行服务，数据库和站点数据不会删除。`,
        start: `确认重新启动分站“${this.site.name}”并恢复域名访问？`,
        unfreeze: `确认解除分站“${this.site.name}”的冻结状态并恢复运行？`,
        backup: `确认立即为分站“${this.site.name}”创建数据库备份？`
      };
      await this.$confirm(descriptions[action] || `确认执行${labels[action] || action}？`, labels[action] || "站点操作", { type: ["stop","freeze"].includes(action) ? "warning" : "info", confirmButtonText: "确认执行", cancelButtonText: "取消" });
      try { await siteAction(this.site.id, action); this.$message.success("任务已进入队列"); this.$router.push("/zhimeng/jobs"); } catch (error) { this.$message.error(error.userMessage || "操作任务创建失败"); }
    },
	async deleteSite() {
	  try {
		const { value } = await this.$prompt(
		  `此操作会永久删除“${this.site.name || this.site.domain}”的容器、数据卷、独立数据库、站点文件、备份和全部配置，并释放域名前缀。请输入完整域名确认：`,
		  "彻底删除站点",
		  {
			confirmButtonText: "永久删除",
			cancelButtonText: "取消",
			type: "error",
			inputPlaceholder: this.site.domain,
			inputValidator: (input) => input === this.site.domain || `请输入完整域名 ${this.site.domain}`
		  }
		);
		await siteAction(this.site.id, "delete", null, value);
		this.$message.success("彻底删除任务已进入队列");
		this.$router.push("/zhimeng/jobs");
	  } catch (error) {
		if (error === "cancel" || error === "close") return;
		this.$message.error(error.userMessage || "彻底删除任务创建失败");
	  }
	},
    async openUpgrade() { this.targetVersion = ""; this.upgradeDialog = true; try { this.versions = (await listVersions()).items || []; } catch (error) { this.$message.error(error.userMessage || "版本列表加载失败"); } },
    async upgrade() { try { await siteAction(this.site.id, "upgrade", this.targetVersion); this.upgradeDialog = false; this.$message.success("升级任务已进入队列"); this.$router.push("/zhimeng/jobs"); } catch (error) { this.$message.error(error.userMessage || "升级任务创建失败"); } }
  }
};
</script>

<style scoped>
.page-header .el-button--text { display: block; margin: -8px 0 4px; }
.site-error { margin-top: 16px; }
.metrics-chart { margin-top: 18px; }
.channel-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 14px; }
.channel-toolbar strong { display: block; color: #1f2d28; font-size: 14px; }
.channel-toolbar span { display: block; margin-top: 4px; color: #87938e; font-size: 12px; }
</style>
