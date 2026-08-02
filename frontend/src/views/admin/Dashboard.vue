<template>
  <div v-loading="loading">
    <PageHeader title="系统概览" description="分站运行、调用趋势与待处理事项">
      <template #actions><el-button icon="el-icon-refresh" @click="load">刷新数据</el-button></template>
    </PageHeader>

    <section class="stat-grid">
      <div v-for="item in metrics" :key="item.label" class="stat-item">
        <div class="stat-item__label">{{ item.label }}</div>
        <div class="stat-item__value">{{ item.display || compactNumber(item.value) }}</div>
        <div class="stat-item__meta">{{ item.meta }}</div>
      </div>
    </section>

    <div class="dashboard-grid">
      <section class="data-panel">
        <div class="panel-header"><div><h2>近 14 日调用</h2><p>所有分站每日调用量汇总</p></div><span class="muted">7 日合计 {{ compactNumber(overview.calls_7d) }}</span></div>
        <div class="panel-body"><div ref="trendChart" class="chart" /></div>
      </section>
      <section class="data-panel">
        <div class="panel-header"><div><h2>待处理事项</h2><p>需要管理员关注的运行状态</p></div></div>
        <div class="panel-body">
          <ul class="attention-list">
            <li v-for="item in attention" :key="item.label">
              <span class="attention-list__name"><i :class="['status-dot', `status-dot--${item.type}`]" />{{ item.label }}</span>
              <span class="attention-list__value">{{ item.value }}</span>
            </li>
          </ul>
        </div>
      </section>
    </div>

    <div class="dashboard-grid">
      <section class="data-panel">
        <div class="panel-header"><div><h2>最近任务</h2><p>开站、升级和运维任务</p></div><el-button type="text" @click="$router.push('/zhimeng/jobs')">查看全部</el-button></div>
        <div class="panel-body panel-body--flush">
          <el-table :data="overview.recent_jobs || []" empty-text="暂无任务">
            <el-table-column prop="site_name" label="分站" min-width="140" show-overflow-tooltip />
            <el-table-column prop="job_type" label="任务" width="100" />
            <el-table-column label="状态" width="100"><template slot-scope="{ row }"><StatusTag :status="row.status" /></template></el-table-column>
            <el-table-column label="进度" min-width="150"><template slot-scope="{ row }"><el-progress :percentage="Number(row.progress || 0)" :show-text="false" /></template></el-table-column>
            <el-table-column label="创建时间" width="170"><template slot-scope="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
          </el-table>
        </div>
      </section>
      <section class="data-panel">
        <div class="panel-header"><div><h2>异常分站</h2><p>告警、离线或开站失败</p></div><el-button type="text" @click="$router.push('/zhimeng/sites')">分站列表</el-button></div>
        <div class="panel-body panel-body--flush">
          <el-table :data="overview.abnormal_sites || []" empty-text="当前没有异常分站" @row-click="openSite">
            <el-table-column prop="name" label="分站" min-width="130" show-overflow-tooltip />
            <el-table-column label="状态" width="90"><template slot-scope="{ row }"><StatusTag :status="row.status" /></template></el-table-column>
            <el-table-column label="最后心跳" width="170"><template slot-scope="{ row }">{{ formatDate(row.last_heartbeat_at) }}</template></el-table-column>
          </el-table>
        </div>
      </section>
    </div>
  </div>
</template>

<script>
import * as echarts from "echarts";
import { dashboard } from "@/api/control";
import PageHeader from "@/components/PageHeader.vue";
import StatusTag from "@/components/StatusTag.vue";
import { compactNumber, formatDate } from "@/utils/format";

export default {
  name: "AdminDashboard",
  components: { PageHeader, StatusTag },
  data() { return { overview: {}, loading: false, chart: null }; },
  computed: {
    metrics() {
      return [
        { label: "分站总数", value: this.overview.sites_total, meta: `${this.overview.sites_online || 0} 个运行中` },
        { label: "今日调用", value: this.overview.calls_today, meta: `近 7 日 ${compactNumber(this.overview.calls_7d)}` },
        { label: "用户总数", value: this.overview.users_total, meta: "来自最新运营快照" },
        { label: "整体成功率", display: `${Number(this.overview.success_rate || 0).toFixed(1)}%`, meta: "近 7 日任务成功率" }
      ];
    },
    attention() {
      return [
        { label: "异常分站", value: this.overview.sites_abnormal || 0, type: "danger" },
        { label: "待升级分站", value: this.overview.pending_upgrades || 0, type: "warning" },
        { label: "执行中任务", value: this.overview.jobs_running || 0, type: "warning" },
        { label: "可用卡密", value: this.overview.codes_unused || 0, type: "success" }
      ];
    }
  },
  async mounted() { window.addEventListener("resize", this.resizeChart); window.addEventListener("control-theme-change", this.themeChanged); await this.load(); },
  beforeDestroy() { window.removeEventListener("resize", this.resizeChart); window.removeEventListener("control-theme-change", this.themeChanged); if (this.chart) this.chart.dispose(); },
  methods: {
    compactNumber, formatDate,
    async load() {
      this.loading = true;
      try { this.overview = (await dashboard()) || {}; this.$nextTick(this.renderChart); }
      catch (error) { this.$message.error(error.userMessage || "概览数据加载失败"); }
      finally { this.loading = false; }
    },
    renderChart() {
      if (!this.$refs.trendChart) return;
      const dark = document.documentElement.dataset.theme === "dark";
      if (!this.chart) this.chart = echarts.init(this.$refs.trendChart, dark ? "dark" : undefined);
      const rows = (this.overview && this.overview.daily) || [];
      this.chart.setOption({
        backgroundColor: "transparent",
        animationDuration: 300,
        color: ["#14764a", "#3d78b8"],
        tooltip: { trigger: "axis" },
        grid: { top: 18, left: 14, right: 14, bottom: 8, containLabel: true },
        xAxis: { type: "category", boundaryGap: false, data: rows.map((row) => String(row.metric_date || "").slice(5)), axisLine: { lineStyle: { color: dark ? "#343c38" : "#dfe6e1" } }, axisTick: { show: false } },
        yAxis: { type: "value", minInterval: 1, splitLine: { lineStyle: { color: dark ? "#272e2a" : "#edf1ee" } } },
        series: [{ name: "调用量", type: "line", smooth: true, symbol: "none", areaStyle: { color: "rgba(20,118,74,.08)" }, data: rows.map((row) => Number(row.calls_total || 0)) }]
      });
    },
    themeChanged() { if (this.chart) { this.chart.dispose(); this.chart = null; } this.$nextTick(this.renderChart); },
    resizeChart() { if (this.chart) this.chart.resize(); },
    openSite(row) { this.$router.push(`/zhimeng/sites/${row.id}`); }
  }
};
</script>
