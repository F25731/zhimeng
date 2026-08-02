<template>
  <div>
    <PageHeader title="分站管理" description="查看每个独立分站的运行、版本和运营快照" />
    <div class="toolbar">
      <el-input v-model.trim="query" prefix-icon="el-icon-search" placeholder="搜索站点名称或域名" clearable @keyup.enter.native="search" @clear="search" />
      <el-select v-model="status" clearable placeholder="全部状态" @change="search">
        <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <span class="toolbar__spacer" />
      <el-button icon="el-icon-refresh" circle title="刷新" @click="load" />
    </div>
    <section class="data-panel">
      <div class="panel-body panel-body--flush">
        <el-table :data="rows" v-loading="loading" empty-text="暂无分站" @row-click="detail">
          <el-table-column label="分站" min-width="190">
            <template slot-scope="{ row }"><div class="link-cell">{{ row.name }}</div><div class="field-hint mono">{{ row.domain }}</div></template>
          </el-table-column>
          <el-table-column label="状态" width="100"><template slot-scope="{ row }"><StatusTag :status="row.status" /></template></el-table-column>
          <el-table-column label="域名路由" width="105"><template slot-scope="{ row }"><el-tag size="small" effect="plain" :type="routeType(row.route_status)">{{ routeLabel(row.route_status) }}</el-tag></template></el-table-column>
          <el-table-column prop="current_version" label="当前版本" width="120"><template slot-scope="{ row }"><span class="mono">{{ row.current_version || '-' }}</span></template></el-table-column>
          <el-table-column prop="users_total" label="用户数" width="100" align="right"><template slot-scope="{ row }">{{ compactNumber(row.users_total) }}</template></el-table-column>
          <el-table-column prop="calls_today" label="今日调用" width="110" align="right"><template slot-scope="{ row }">{{ compactNumber(row.calls_today) }}</template></el-table-column>
          <el-table-column prop="calls_7d" label="近 7 日调用" width="120" align="right"><template slot-scope="{ row }">{{ compactNumber(row.calls_7d) }}</template></el-table-column>
          <el-table-column label="最后心跳" width="175"><template slot-scope="{ row }">{{ formatDate(row.last_heartbeat_at) }}</template></el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template slot-scope="{ row }">
              <div class="table-actions">
                <el-tooltip content="查看详情" placement="top"><el-button class="table-action" icon="el-icon-view" circle aria-label="查看详情" @click.stop="detail(row)" /></el-tooltip>
                <el-tooltip content="访问分站" placement="top"><el-button class="table-action" icon="el-icon-top-right" circle aria-label="访问分站" @click.stop="open(row)" /></el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div class="pagination-bar"><el-pagination background layout="total, sizes, prev, pager, next" :page-sizes="[20,50,100]" :page-size="pageSize" :total="total" :current-page="page" @size-change="changeSize" @current-change="changePage" /></div>
    </section>
  </div>
</template>

<script>
import { listSites } from "@/api/control";
import PageHeader from "@/components/PageHeader.vue";
import StatusTag from "@/components/StatusTag.vue";
import { compactNumber, formatDate, statusLabels } from "@/utils/format";

export default {
  name: "SitesPage",
  components: { PageHeader, StatusTag },
  data() {
    return {
      rows: [], total: 0, page: 1, pageSize: 20, query: "", status: "", loading: false,
      statuses: ["pending","provisioning","active","warning","offline","stopped","frozen","upgrading","failed"].map((value) => ({ value, label: statusLabels[value] }))
    };
  },
  created() { this.load(); },
  methods: {
    compactNumber, formatDate,
    routeLabel(value) { return ({ disabled: "未开放", unverified: "待验证", activating: "开放中", verifying_https: "验 HTTPS", active: "已接入", failed: "异常" })[value] || "未检测"; },
    routeType(value) { return value === "active" ? "success" : value === "failed" ? "danger" : value === "disabled" ? "info" : "warning"; },
    async load() {
      this.loading = true;
      try { const data = await listSites({ page: this.page, pageSize: this.pageSize, query: this.query, status: this.status }); this.rows = data.items || []; this.total = data.total || 0; }
      catch (error) { this.$message.error(error.userMessage || "分站列表加载失败"); }
      finally { this.loading = false; }
    },
    search() { this.page = 1; this.load(); },
    changePage(value) { this.page = value; this.load(); },
    changeSize(value) { this.pageSize = value; this.page = 1; this.load(); },
    detail(row) { this.$router.push(`/zhimeng/sites/${row.id}`); },
    open(row) { window.open(`https://${row.domain}`, "_blank", "noopener"); }
  }
};
</script>
