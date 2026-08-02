<template>
  <div>
    <PageHeader title="部署节点" description="Agent 节点资源与心跳状态">
      <template #actions><el-button icon="el-icon-refresh" @click="load">刷新状态</el-button></template>
    </PageHeader>
    <section class="data-panel">
      <div class="panel-body panel-body--flush">
        <el-table :data="rows" v-loading="loading" empty-text="暂无部署节点">
          <el-table-column label="节点" min-width="170"><template slot-scope="{ row }"><div class="link-cell">{{ row.name }}</div><div class="field-hint mono">{{ row.public_ip || '未设置公网 IP' }}</div></template></el-table-column>
          <el-table-column label="状态" width="100"><template slot-scope="{ row }"><StatusTag :status="nodeStatus(row)" /></template></el-table-column>
          <el-table-column label="CPU" width="90"><template slot-scope="{ row }">{{ row.cpu_total || '-' }} 核</template></el-table-column>
          <el-table-column label="内存" width="120"><template slot-scope="{ row }">{{ formatMemory(row.memory_total_mb) }}</template></el-table-column>
          <el-table-column label="磁盘" width="100"><template slot-scope="{ row }">{{ row.disk_total_gb ? `${row.disk_total_gb} GB` : '-' }}</template></el-table-column>
          <el-table-column prop="site_count" label="分站数" width="90" align="right" />
          <el-table-column prop="agent_version" label="Agent 版本" width="120"><template slot-scope="{ row }"><span class="mono">{{ row.agent_version || '-' }}</span></template></el-table-column>
          <el-table-column prop="docker_version" label="Docker" width="120"><template slot-scope="{ row }"><span class="mono">{{ row.docker_version || '-' }}</span></template></el-table-column>
          <el-table-column label="最后心跳" width="175"><template slot-scope="{ row }">{{ formatDate(row.last_heartbeat_at) }}</template></el-table-column>
        </el-table>
      </div>
    </section>
  </div>
</template>

<script>
import { listNodes } from "@/api/control";
import PageHeader from "@/components/PageHeader.vue";
import StatusTag from "@/components/StatusTag.vue";
import { formatDate } from "@/utils/format";

export default {
  name: "NodesPage", components: { PageHeader, StatusTag },
  data() { return { rows: [], loading: false }; },
  created() { this.load(); },
  methods: {
    formatDate,
    async load() { this.loading = true; try { this.rows = (await listNodes()).items || []; } catch (error) { this.$message.error(error.userMessage || "节点列表加载失败"); } finally { this.loading = false; } },
    nodeStatus(row) { if (!row.last_heartbeat_at || Date.now() - new Date(row.last_heartbeat_at).getTime() > 120000) return "offline"; return row.status; },
    formatMemory(value) { return value ? value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${value} MB` : "-"; }
  }
};
</script>
