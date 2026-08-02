<template>
  <div>
    <PageHeader title="版本管理" description="登记分站镜像并控制可用的升级版本">
      <template #actions><el-button type="primary" icon="el-icon-plus" @click="openCreate">登记版本</el-button></template>
    </PageHeader>
    <section class="data-panel">
      <div class="panel-body panel-body--flush">
        <el-table :data="rows" v-loading="loading" empty-text="暂无版本">
          <el-table-column label="版本" width="130"><template slot-scope="{ row }"><span class="mono link-cell">{{ row.version }}</span></template></el-table-column>
          <el-table-column prop="image" label="应用镜像" min-width="260" show-overflow-tooltip><template slot-scope="{ row }"><span class="mono">{{ row.image }}</span></template></el-table-column>
          <el-table-column label="通道" width="100"><template slot-scope="{ row }"><el-tag size="small" effect="plain">{{ channelLabel(row.channel) }}</el-tag></template></el-table-column>
          <el-table-column label="状态" width="100"><template slot-scope="{ row }"><StatusTag :status="row.status" /></template></el-table-column>
          <el-table-column prop="migration_version" label="迁移版本" width="130"><template slot-scope="{ row }"><span class="mono">{{ row.migration_version || '-' }}</span></template></el-table-column>
          <el-table-column label="强制升级" width="90"><template slot-scope="{ row }">{{ row.force_upgrade ? '是' : '否' }}</template></el-table-column>
          <el-table-column label="发布时间" width="170"><template slot-scope="{ row }">{{ formatDate(row.published_at) }}</template></el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template slot-scope="{ row }">
              <div class="table-actions">
                <el-tooltip v-if="row.status === 'draft'" content="发布版本" placement="top"><el-button class="table-action" icon="el-icon-check" circle aria-label="发布版本" @click="publish(row)" /></el-tooltip>
                <el-tooltip content="查看说明" placement="top"><el-button class="table-action" icon="el-icon-document" circle aria-label="查看说明" @click="showNotes(row)" /></el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </section>

    <el-dialog title="登记新版本" :visible.sync="dialog" width="560px" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-row :gutter="16"><el-col :span="12"><el-form-item label="版本号" required><el-input v-model.trim="form.version" placeholder="例如 v1.4.0" /></el-form-item></el-col><el-col :span="12"><el-form-item label="发布通道"><el-select v-model="form.channel" style="width:100%"><el-option label="稳定版" value="stable" /><el-option label="测试版" value="beta" /><el-option label="灰度版" value="canary" /></el-select></el-form-item></el-col></el-row>
        <el-form-item label="应用镜像" required><el-input v-model.trim="form.image" class="mono-input" placeholder="ghcr.io/example/site-app:v1.0.0" /></el-form-item>
        <el-row :gutter="16"><el-col :span="12"><el-form-item label="数据库迁移版本"><el-input v-model.trim="form.migrationVersion" /></el-form-item></el-col><el-col :span="12"><el-form-item label="最低可升级版本"><el-input v-model.trim="form.minUpgradeVersion" /></el-form-item></el-col></el-row>
        <el-form-item label="升级说明"><el-input v-model="form.releaseNotes" type="textarea" :rows="4" maxlength="4000" show-word-limit /></el-form-item>
        <el-form-item><el-checkbox v-model="form.forceUpgrade">标记为强制升级版本</el-checkbox></el-form-item>
      </el-form>
      <span slot="footer"><el-button @click="dialog=false">取消</el-button><el-button type="primary" :loading="creating" :disabled="!form.version || !form.image" @click="create">保存草稿</el-button></span>
    </el-dialog>
  </div>
</template>

<script>
import { createVersion, listVersions, publishVersion } from "@/api/control";
import PageHeader from "@/components/PageHeader.vue";
import StatusTag from "@/components/StatusTag.vue";
import { formatDate } from "@/utils/format";

export default {
  name: "VersionsPage",
  components: { PageHeader, StatusTag },
  data() { return { rows: [], loading: false, dialog: false, creating: false, form: this.emptyForm() }; },
  created() { this.load(); },
  methods: {
    formatDate,
    emptyForm() { return { version: "", image: "", channel: "stable", releaseNotes: "", migrationVersion: "", minUpgradeVersion: "", forceUpgrade: false }; },
    channelLabel(value) { return { stable: "稳定", beta: "测试", canary: "灰度" }[value] || value; },
    async load() { this.loading = true; try { this.rows = (await listVersions()).items || []; } catch (error) { this.$message.error(error.userMessage || "版本列表加载失败"); } finally { this.loading = false; } },
    openCreate() { this.form = this.emptyForm(); this.dialog = true; },
    async create() { this.creating = true; try { await createVersion(this.form); this.dialog = false; this.$message.success("版本草稿已保存"); this.load(); } catch (error) { this.$message.error(error.userMessage || "版本登记失败"); } finally { this.creating = false; } },
    async publish(row) { await this.$confirm(`发布版本 ${row.version} 后可用于分站升级，确认继续？`, "发布版本", { type: "warning" }); try { await publishVersion(row.id); this.$message.success("版本已发布"); this.load(); } catch (error) { this.$message.error(error.userMessage || "版本发布失败"); } },
    showNotes(row) { this.$alert(row.release_notes || "该版本没有升级说明。", `${row.version} 升级说明`, { confirmButtonText: "关闭" }); }
  }
};
</script>
