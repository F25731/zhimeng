<template>
  <div>
    <PageHeader title="操作日志" description="管理员关键操作的不可变审计记录" />
    <div class="toolbar">
      <el-input v-model.trim="query" prefix-icon="el-icon-search" placeholder="管理员、对象 ID 或 IP" clearable @keyup.enter.native="search" @clear="search" />
      <el-select v-model="action" clearable placeholder="全部模块" @change="search"><el-option label="管理员认证" value="admin." /><el-option label="卡密管理" value="codes." /><el-option label="分站操作" value="sites." /><el-option label="任务操作" value="jobs." /><el-option label="版本管理" value="versions." /></el-select>
      <span class="toolbar__spacer" /><el-button icon="el-icon-refresh" circle @click="load" />
    </div>
    <section class="data-panel">
      <div class="panel-body panel-body--flush">
        <el-table :data="rows" v-loading="loading" empty-text="暂无操作日志" @row-click="showDetail">
          <el-table-column label="时间" width="175"><template slot-scope="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
          <el-table-column prop="admin_username" label="管理员" width="120"><template slot-scope="{ row }">{{ row.admin_username || '系统' }}</template></el-table-column>
          <el-table-column label="操作" min-width="180"><template slot-scope="{ row }"><span class="mono">{{ row.action }}</span></template></el-table-column>
          <el-table-column prop="target_type" label="对象类型" width="120" />
          <el-table-column label="对象 ID" min-width="180" show-overflow-tooltip><template slot-scope="{ row }"><span class="mono">{{ row.target_id || '-' }}</span></template></el-table-column>
          <el-table-column prop="ip" label="来源 IP" width="140"><template slot-scope="{ row }"><span class="mono">{{ row.ip || '-' }}</span></template></el-table-column>
        </el-table>
      </div>
      <div class="pagination-bar"><el-pagination background layout="total, sizes, prev, pager, next" :page-sizes="[20,50,100]" :page-size="pageSize" :total="total" :current-page="page" @size-change="changeSize" @current-change="changePage" /></div>
    </section>
  </div>
</template>

<script>
import { listAudit } from "@/api/control";
import PageHeader from "@/components/PageHeader.vue";
import { formatDate } from "@/utils/format";

export default {
  name: "AuditPage", components: { PageHeader },
  data() { return { rows: [], total: 0, page: 1, pageSize: 20, query: "", action: "", loading: false }; },
  created() { this.load(); },
  methods: {
    formatDate,
    async load() { this.loading = true; try { const data = await listAudit({ page: this.page, pageSize: this.pageSize, query: this.query, action: this.action }); this.rows = data.items || []; this.total = data.total || 0; } catch (error) { this.$message.error(error.userMessage || "操作日志加载失败"); } finally { this.loading = false; } },
    search() { this.page = 1; this.load(); }, changePage(value) { this.page = value; this.load(); }, changeSize(value) { this.pageSize = value; this.page = 1; this.load(); },
    showDetail(row) { let detail = row.detail_json || {}; if (typeof detail === "string") { try { detail = JSON.parse(detail); } catch (_) { detail = {}; } } this.$alert(`<pre style="white-space:pre-wrap;word-break:break-all;margin:0">${this.escapeHTML(JSON.stringify(detail, null, 2))}</pre>`, "操作详情", { dangerouslyUseHTMLString: true, confirmButtonText: "关闭" }); },
    escapeHTML(value) { return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;" }[char])); }
  }
};
</script>
