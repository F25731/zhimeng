<template>
  <div>
    <PageHeader title="开站卡密" description="创建和管理客户开站资格，完整卡密只在创建时显示一次">
      <template #actions><el-button type="primary" icon="el-icon-plus" @click="openCreate">创建卡密</el-button></template>
    </PageHeader>
    <div class="toolbar">
      <el-input v-model.trim="query" prefix-icon="el-icon-search" placeholder="搜索备注或卡密前缀" clearable @keyup.enter.native="search" @clear="search" />
      <el-select v-model="status" clearable placeholder="全部状态" @change="search">
        <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <span class="toolbar__spacer" />
      <el-button icon="el-icon-download" @click="exportCodes">导出列表</el-button>
      <el-button icon="el-icon-refresh" circle title="刷新" @click="load" />
    </div>
    <section class="data-panel">
      <div class="panel-body panel-body--flush">
        <el-table :data="rows" v-loading="loading" empty-text="暂无卡密">
          <el-table-column prop="prefix" label="卡密前缀" width="150"><template slot-scope="{ row }"><span class="mono">{{ row.prefix }}••••</span></template></el-table-column>
          <el-table-column prop="remark" label="客户备注" min-width="170" show-overflow-tooltip />
          <el-table-column label="状态" width="105"><template slot-scope="{ row }"><StatusTag :status="row.status" /></template></el-table-column>
          <el-table-column label="开站额度" width="100"><template slot-scope="{ row }">{{ row.usedSites || 0 }} / {{ row.maxSites || 1 }}</template></el-table-column>
          <el-table-column prop="initialVersion" label="初始版本" width="115"><template slot-scope="{ row }"><span class="mono">{{ row.initialVersion || 'latest' }}</span></template></el-table-column>
          <el-table-column label="有效期" width="170"><template slot-scope="{ row }">{{ row.expiresAt ? formatDate(row.expiresAt) : '长期有效' }}</template></el-table-column>
          <el-table-column label="创建时间" width="170"><template slot-scope="{ row }">{{ formatDate(row.createdAt) }}</template></el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template slot-scope="{ row }">
              <div class="table-actions">
                <el-tooltip v-if="['unused','reserved','failed'].includes(row.status)" content="撤销卡密" placement="top">
                  <el-button class="table-action" icon="el-icon-close" circle aria-label="撤销卡密" @click="revoke(row)" />
                </el-tooltip>
                <el-tooltip v-if="row.status === 'unused'" content="删除卡密" placement="top">
                  <el-button class="table-action table-action--danger" icon="el-icon-delete" circle aria-label="删除卡密" @click="remove(row)" />
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div class="pagination-bar"><el-pagination background layout="total, sizes, prev, pager, next" :page-sizes="[20,50,100]" :page-size="pageSize" :total="total" :current-page="page" @size-change="changeSize" @current-change="changePage" /></div>
    </section>

    <el-dialog title="创建开站卡密" :visible.sync="dialog" width="500px" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="客户备注" required><el-input v-model.trim="form.remark" maxlength="100" placeholder="例如客户名称、订单号或用途" /></el-form-item>
        <el-row :gutter="16">
          <el-col :span="12"><el-form-item label="创建数量"><el-input-number v-model="form.quantity" controls-position="right" :min="1" :max="1000" style="width:100%" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="每张可开站数"><el-input-number v-model="form.maxSites" controls-position="right" :min="1" :max="100" style="width:100%" /></el-form-item></el-col>
        </el-row>
        <el-form-item label="初始版本"><el-select v-model="form.initialVersion" filterable allow-create default-first-option style="width:100%"><el-option label="最新稳定版" value="latest" /><el-option v-for="version in versions" :key="version.version" :label="version.version" :value="version.version" /></el-select></el-form-item>
        <el-form-item label="过期时间"><el-date-picker v-model="form.expiresAt" type="datetime" placeholder="不选择则长期有效" style="width:100%" :picker-options="pickerOptions" /></el-form-item>
      </el-form>
      <span slot="footer"><el-button @click="dialog=false">取消</el-button><el-button type="primary" :loading="creating" :disabled="!form.remark" @click="create">创建</el-button></span>
    </el-dialog>

    <el-dialog title="卡密创建成功" :visible.sync="resultDialog" width="620px" :close-on-click-modal="false" :show-close="false">
      <el-alert type="warning" title="完整卡密仅显示这一次，请立即复制并安全交付给客户。" :closable="false" show-icon />
      <el-input :value="createdCodes.join('\n')" type="textarea" :rows="Math.min(12, Math.max(4, createdCodes.length + 1))" readonly class="mono result-codes" />
      <span slot="footer"><el-button icon="el-icon-document-copy" @click="copyCodes">复制全部</el-button><el-button type="primary" @click="finishCreate">我已保存</el-button></span>
    </el-dialog>
  </div>
</template>

<script>
import { createCodes, deleteCode, listCodes, listVersions, revokeCode } from "@/api/control";
import PageHeader from "@/components/PageHeader.vue";
import StatusTag from "@/components/StatusTag.vue";
import { formatDate, statusLabels } from "@/utils/format";

export default {
  name: "CardsPage",
  components: { PageHeader, StatusTag },
  data() {
    return {
      rows: [], total: 0, page: 1, pageSize: 20, query: "", status: "", loading: false,
      dialog: false, resultDialog: false, creating: false, createdCodes: [], versions: [],
      form: { remark: "", quantity: 1, maxSites: 1, initialVersion: "latest", expiresAt: null },
      statuses: ["unused","reserved","provisioning","active","failed","revoked","expired"].map((value) => ({ value, label: statusLabels[value] })),
      pickerOptions: { disabledDate: (date) => date.getTime() < Date.now() - 86400000 }
    };
  },
  created() { this.load(); },
  methods: {
    formatDate,
    async load() {
      this.loading = true;
      try {
        const data = await listCodes({ page: this.page, pageSize: this.pageSize, query: this.query, status: this.status });
        this.rows = data.items || []; this.total = data.total || 0;
      } catch (error) { this.$message.error(error.userMessage || "卡密列表加载失败"); }
      finally { this.loading = false; }
    },
    search() { this.page = 1; this.load(); },
    changePage(value) { this.page = value; this.load(); },
    changeSize(value) { this.pageSize = value; this.page = 1; this.load(); },
    async openCreate() {
      this.dialog = true;
      try { this.versions = (await listVersions()).items.filter((item) => item.status === "published"); } catch (_) { this.versions = []; }
    },
    async create() {
      this.creating = true;
      try {
        const payload = { ...this.form, expiresAt: this.form.expiresAt ? this.form.expiresAt.toISOString() : null };
        const data = await createCodes(payload);
        this.createdCodes = data.codes || [];
        this.dialog = false;
        this.resultDialog = true;
      } catch (error) { this.$message.error(error.userMessage || "卡密创建失败"); }
      finally { this.creating = false; }
    },
    async copyCodes() { await navigator.clipboard.writeText(this.createdCodes.join("\n")); this.$message.success("全部卡密已复制"); },
    finishCreate() { this.resultDialog = false; this.form = { remark: "", quantity: 1, maxSites: 1, initialVersion: "latest", expiresAt: null }; this.load(); },
    async revoke(row) { await this.$confirm(`确认撤销卡密 ${row.prefix}••••？`, "撤销卡密", { type: "warning" }); await revokeCode(row.id); this.$message.success("卡密已撤销"); this.load(); },
    async remove(row) { await this.$confirm("删除后无法恢复，确认继续？", "删除卡密", { type: "warning" }); await deleteCode(row.id); this.$message.success("卡密已删除"); this.load(); },
    exportCodes() { window.open("/api/admin/codes/export", "_blank", "noopener"); }
  }
};
</script>

<style scoped>
.result-codes { margin-top: 16px; }
</style>
