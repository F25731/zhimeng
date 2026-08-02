<template>
  <div>
    <div v-if="!siteId" class="toolbar">
      <el-input v-model.trim="query" prefix-icon="el-icon-search" placeholder="任务 ID、站点或域名" clearable @keyup.enter.native="search" @clear="search" />
      <el-select v-model="status" clearable placeholder="全部状态" @change="search"><el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" /></el-select>
      <el-select v-model="type" clearable placeholder="全部类型" @change="search"><el-option v-for="item in types" :key="item.value" :label="item.label" :value="item.value" /></el-select>
      <span class="toolbar__spacer" /><el-button icon="el-icon-refresh" circle @click="load" />
    </div>
    <section class="data-panel">
      <div class="panel-body panel-body--flush">
        <el-table :data="rows" v-loading="loading" empty-text="暂无任务" @row-click="openDetail">
          <el-table-column label="任务" min-width="160"><template slot-scope="{ row }"><div class="link-cell">{{ typeLabel(row.job_type) }}</div><div class="field-hint mono">{{ String(row.id).slice(0, 13) }}…</div></template></el-table-column>
          <el-table-column v-if="!siteId" label="分站" min-width="150" show-overflow-tooltip><template slot-scope="{ row }"><div>{{ row.site_name || '-' }}</div><div class="field-hint mono">{{ row.domain || '-' }}</div></template></el-table-column>
          <el-table-column label="状态" width="105"><template slot-scope="{ row }"><StatusTag :status="row.status" /></template></el-table-column>
          <el-table-column label="当前步骤" min-width="130"><template slot-scope="{ row }">{{ stepLabel(row.current_step) }}</template></el-table-column>
          <el-table-column label="进度" min-width="140"><template slot-scope="{ row }"><el-progress :percentage="Number(row.progress || 0)" :status="row.status === 'failed' ? 'exception' : row.status === 'completed' ? 'success' : undefined" /></template></el-table-column>
          <el-table-column label="尝试" width="70" align="center"><template slot-scope="{ row }">{{ row.attempt || 0 }} / {{ row.max_attempts || 3 }}</template></el-table-column>
          <el-table-column label="创建时间" width="170"><template slot-scope="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template slot-scope="{ row }">
              <div class="table-actions">
                <el-tooltip v-if="['failed','manual_intervention'].includes(row.status) && row.retryable" content="重试任务" placement="top"><el-button class="table-action" icon="el-icon-refresh" circle aria-label="重试任务" @click.stop="retry(row)" /></el-tooltip>
                <el-tooltip content="查看详情" placement="top"><el-button class="table-action" icon="el-icon-view" circle aria-label="查看详情" @click.stop="openDetail(row)" /></el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div v-if="!siteId" class="pagination-bar"><el-pagination background layout="total, sizes, prev, pager, next" :page-sizes="[20,50,100]" :page-size="pageSize" :total="total" :current-page="page" @size-change="changeSize" @current-change="changePage" /></div>
    </section>

    <el-drawer title="任务详情" :visible.sync="drawer" size="520px" direction="rtl">
      <div class="job-drawer" v-if="selected.id">
        <div class="detail-grid">
          <div class="detail-item"><div class="detail-item__label">任务 ID</div><div class="detail-item__value mono">{{ selected.id }}</div></div>
          <div class="detail-item"><div class="detail-item__label">状态</div><StatusTag :status="selected.status" /></div>
          <div class="detail-item"><div class="detail-item__label">任务类型</div><div class="detail-item__value">{{ typeLabel(selected.jobType || selected.job_type) }}</div></div>
          <div class="detail-item"><div class="detail-item__label">当前步骤</div><div class="detail-item__value">{{ stepLabel(selected.currentStep || selected.current_step) }}</div></div>
          <div class="detail-item"><div class="detail-item__label">尝试次数</div><div class="detail-item__value">{{ selected.attempt || 0 }}</div></div>
          <div class="detail-item"><div class="detail-item__label">更新时间</div><div class="detail-item__value">{{ formatDate(selected.updatedAt || selected.updated_at) }}</div></div>
        </div>
        <el-alert v-if="selected.errorMessage || selected.error_message" class="job-error" type="error" :title="selected.errorMessage || selected.error_message" :description="selected.errorCode || selected.error_code" :closable="false" show-icon />
      </div>
    </el-drawer>
  </div>
</template>

<script>
import { getAdminJob, listJobs, retryJob } from "@/api/control";
import StatusTag from "@/components/StatusTag.vue";
import { formatDate, statusLabels } from "@/utils/format";

const jobTypes = { provision: "创建分站", start: "启动", stop: "停止", restart: "重启", freeze: "冻结", unfreeze: "解除冻结", backup: "备份", upgrade: "升级", delete: "彻底删除" };
const stepNames = { pending: "等待执行", validating: "校验参数", allocating_node: "分配节点", generating_secrets: "生成密钥", creating_database: "创建数据库", generating_config: "生成配置", pulling_image: "准备镜像", starting_containers: "启动容器", initializing_database: "初始化数据", creating_admin: "创建管理员", applying_branding: "应用品牌", checking_health: "健康检查", processing: "执行操作", backing_up: "数据库备份", upgrading: "升级应用", stopping_runtime: "关闭运行服务", deleting_database: "删除独立数据库", deleting_files: "删除站点文件", releasing_domain: "释放域名前缀", completed: "已完成", active: "已激活", failed: "失败" };

export default {
  name: "JobsTable",
  components: { StatusTag },
  props: { siteId: { type: String, default: "" } },
  data() {
    return {
      rows: [], total: 0, page: 1, pageSize: this.siteId ? 100 : 20, query: "", status: "", type: "", loading: false, drawer: false, selected: {},
      statuses: ["pending","running","completed","failed","manual_intervention"].map((value) => ({ value, label: statusLabels[value] })),
      types: Object.entries(jobTypes).map(([value, label]) => ({ value, label }))
    };
  },
  created() { this.load(); },
  methods: {
    formatDate,
    typeLabel(value) { return jobTypes[value] || value || "未知任务"; },
    stepLabel(value) { return stepNames[value] || value || "-"; },
    async load() {
      this.loading = true;
      try { const data = await listJobs({ page: this.page, pageSize: this.pageSize, query: this.query, status: this.status, type: this.type, siteId: this.siteId }); this.rows = data.items || []; this.total = data.total || 0; }
      catch (error) { this.$message.error(error.userMessage || "任务列表加载失败"); }
      finally { this.loading = false; }
    },
    search() { this.page = 1; this.load(); },
    changePage(value) { this.page = value; this.load(); },
    changeSize(value) { this.pageSize = value; this.page = 1; this.load(); },
    async retry(row) { await this.$confirm("确认重新执行此任务？系统会从幂等步骤继续。", "重试任务", { type: "warning" }); try { await retryJob(row.id); this.$message.success("任务已重新进入队列"); this.load(); } catch (error) { this.$message.error(error.userMessage || "任务无法重试"); } },
    async openDetail(row) { this.drawer = true; try { this.selected = (await getAdminJob(row.id)).job; } catch (_) { this.selected = row; } }
  }
};
</script>

<style scoped>
.job-drawer { padding: 0 20px 24px; }
.job-error { margin-top: 16px; }
</style>
