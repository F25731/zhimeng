<template>
  <div class="public-shell">
    <header class="public-header">
      <div class="public-brand"><img class="brand-logo" src="/logo.svg" alt="" /><span>分站开通</span></div>
      <div class="public-header__meta">{{ rootDomain }}</div>
    </header>
    <main class="public-main">
      <section class="provision-panel">
        <aside class="provision-sidebar">
          <h1>创建分站</h1>
          <p>填写开站资料后，系统会自动完成数据库、应用与任务服务的独立部署。</p>
          <el-steps direction="vertical" :active="step" finish-status="success" :space="52">
            <el-step title="验证卡密" />
            <el-step title="域名与备注" />
            <el-step title="公开资料" />
            <el-step title="管理员" />
            <el-step title="确认创建" />
          </el-steps>
        </aside>

        <div class="provision-body">
          <div class="provision-body__head">
            <h2>{{ headings[step].title }}</h2>
            <p>{{ headings[step].description }}</p>
          </div>

          <el-form v-if="step === 0" class="provision-form" label-position="top" @submit.native.prevent="verify">
            <el-form-item label="开站卡密">
              <el-input v-model.trim="code" class="mono-input" placeholder="SITE-XXXX-XXXX-XXXX-XXXX" maxlength="24" autofocus @keyup.enter.native="verify" />
            </el-form-item>
            <p class="field-hint">卡密验证后会保留 30 分钟填写时间。</p>
            <div class="form-actions"><el-button type="primary" :loading="loading" :disabled="!code" @click="verify">验证卡密</el-button></div>
          </el-form>

          <el-form v-else-if="step === 1" class="provision-form" label-position="top">
            <el-form-item label="分站域名" :error="domainError">
              <el-input v-model.trim="form.prefix" class="domain-input" placeholder="例如 brand-a" maxlength="32" @input="scheduleDomainCheck">
                <template slot="append">.{{ rootDomain }}</template>
              </el-input>
              <p v-if="domainChecking" class="field-hint"><i class="el-icon-loading" /> 正在检查域名</p>
              <p v-else-if="domainConfirmed" class="field-hint"><i class="el-icon-success" /> 域名可用，已为当前卡密保留 20 分钟</p>
            </el-form-item>
            <el-form-item label="分站备注（仅总控可见）">
              <el-input v-model.trim="form.name" maxlength="128" show-word-limit placeholder="用于总控识别，不会显示给分站客户" />
            </el-form-item>
            <div class="form-actions"><el-button @click="step = 0">上一步</el-button><el-button type="primary" :disabled="!canContinueSite" @click="step = 2">下一步</el-button></div>
          </el-form>

          <el-form v-else-if="step === 2" class="provision-form provision-form--wide" label-position="top">
            <section class="profile-section">
              <h3>品牌信息</h3>
              <div class="profile-grid">
                <el-form-item label="网站标题（可选）">
                  <el-input v-model="form.siteProfile.title" maxlength="40" show-word-limit placeholder="不填写则不显示标题" />
                </el-form-item>
                <div />
                <el-form-item label="网站 Logo（可选）">
                  <el-input v-model.trim="assetSources.logoUrl" class="asset-url-input" placeholder="粘贴 PNG、JPEG、WebP 或 SVG 图片地址" @keyup.enter.native="importAsset('logoUrl')">
                    <el-button slot="append" icon="el-icon-download" :loading="importing.logoUrl" @click="importAsset('logoUrl')">下载并使用</el-button>
                  </el-input>
                  <div class="logo-row">
                    <img v-if="form.siteProfile.logoUrl" :src="form.siteProfile.logoUrl" class="logo-preview" alt="网站 Logo" />
                    <el-upload action="#" accept="image/png,image/jpeg,image/webp" :show-file-list="false" :before-upload="file => uploadAsset(file, 'logoUrl')">
                      <el-button icon="el-icon-upload2" :loading="uploading.logoUrl">{{ form.siteProfile.logoUrl ? "重新上传" : "上传 Logo" }}</el-button>
                    </el-upload>
                    <el-button v-if="form.siteProfile.logoUrl" type="text" @click="form.siteProfile.logoUrl = ''">移除</el-button>
                  </div>
                </el-form-item>
                <el-form-item label="浏览器图标（可选）">
                  <el-input v-model.trim="assetSources.iconUrl" class="asset-url-input" placeholder="粘贴 PNG、JPEG、WebP 或 SVG 图片地址" @keyup.enter.native="importAsset('iconUrl')">
                    <el-button slot="append" icon="el-icon-download" :loading="importing.iconUrl" @click="importAsset('iconUrl')">下载并使用</el-button>
                  </el-input>
                  <div class="logo-row">
                    <img v-if="form.siteProfile.iconUrl" :src="form.siteProfile.iconUrl" class="icon-preview" alt="浏览器图标" />
                    <el-upload action="#" accept="image/png,image/jpeg,image/webp" :show-file-list="false" :before-upload="file => uploadAsset(file, 'iconUrl')">
                      <el-button icon="el-icon-upload2" :loading="uploading.iconUrl">{{ form.siteProfile.iconUrl ? "重新上传" : "上传图标" }}</el-button>
                    </el-upload>
                    <el-button v-if="form.siteProfile.iconUrl" type="text" @click="form.siteProfile.iconUrl = ''">移除</el-button>
                  </div>
                </el-form-item>
              </div>
            </section>

            <section class="profile-section">
              <h3>SEO 信息</h3>
              <el-form-item label="SEO 标题（可选）"><el-input v-model="form.siteProfile.seoTitle" maxlength="72" show-word-limit /></el-form-item>
              <el-form-item label="SEO 描述（可选）"><el-input v-model="form.siteProfile.seoDescription" type="textarea" :rows="3" maxlength="180" show-word-limit /></el-form-item>
              <el-form-item label="SEO 关键词（可选）"><el-input v-model="form.siteProfile.seoKeywords" maxlength="240" placeholder="多个关键词使用逗号分隔" /></el-form-item>
            </section>

            <section class="profile-section">
              <h3>页脚与联系方式</h3>
              <el-form-item label="版权信息（可选）"><el-input v-model="form.siteProfile.footerCopyright" maxlength="120" show-word-limit /></el-form-item>
              <div class="profile-grid">
                <el-form-item label="使用条款链接（可选）"><el-input v-model.trim="form.siteProfile.termsUrl" placeholder="https:// 或站内 /terms" /></el-form-item>
                <el-form-item label="隐私政策链接（可选）"><el-input v-model.trim="form.siteProfile.privacyUrl" placeholder="https:// 或站内 /privacy" /></el-form-item>
                <el-form-item label="邮箱链接（可选）"><el-input v-model.trim="form.siteProfile.socials.email.url" placeholder="mailto:name@example.com" /></el-form-item>
                <el-form-item label="Telegram（可选）"><el-input v-model.trim="form.siteProfile.socials.telegram.url" placeholder="https://t.me/..." /></el-form-item>
                <el-form-item label="X（可选）"><el-input v-model.trim="form.siteProfile.socials.x.url" placeholder="https://x.com/..." /></el-form-item>
                <el-form-item label="Instagram（可选）"><el-input v-model.trim="form.siteProfile.socials.instagram.url" placeholder="https://instagram.com/..." /></el-form-item>
              </div>
            </section>

            <section class="profile-section">
              <div class="profile-section__head">
                <h3>友情链接</h3>
                <el-button icon="el-icon-plus" :disabled="form.siteProfile.friendLinks.length >= 12" @click="addFriendLink">添加链接</el-button>
              </div>
              <p v-if="!form.siteProfile.friendLinks.length" class="empty-profile-field">未添加时，分站不会显示友情链接区域。</p>
              <div v-for="(link, index) in form.siteProfile.friendLinks" :key="index" class="friend-link-row">
                <el-input v-model="link.label" maxlength="32" placeholder="链接名称" />
                <el-input v-model.trim="link.url" placeholder="https://..." />
                <el-button icon="el-icon-delete" circle title="删除" @click="form.siteProfile.friendLinks.splice(index, 1)" />
              </div>
            </section>

            <p class="field-hint">以上资料全部可选。未填写的内容会保持空白，分站不会使用任何原项目资料代替。</p>
            <div class="form-actions"><el-button @click="step = 1">上一步</el-button><el-button type="primary" @click="step = 3">下一步</el-button></div>
          </el-form>

          <el-form v-else-if="step === 3" class="provision-form" label-position="top" @submit.native.prevent="step = canContinueAdmin ? 4 : 3">
            <el-form-item label="管理员用户名"><el-input v-model.trim="form.adminUsername" maxlength="32" autocomplete="username" placeholder="4-32 位字符" /></el-form-item>
            <el-form-item label="管理员密码"><el-input v-model="form.adminPassword" type="password" show-password autocomplete="new-password" placeholder="至少 10 位，包含字母和数字" /></el-form-item>
            <el-form-item label="确认密码" :error="passwordError"><el-input v-model="confirmPassword" type="password" show-password autocomplete="new-password" /></el-form-item>
            <div class="form-actions"><el-button @click="step = 2">上一步</el-button><el-button type="primary" :disabled="!canContinueAdmin" @click="step = 4">下一步</el-button></div>
          </el-form>

          <div v-else class="provision-form provision-form--wide">
            <dl class="confirm-list">
              <div class="confirm-row"><dt>分站地址</dt><dd class="mono">https://{{ form.prefix }}.{{ rootDomain }}</dd></div>
              <div class="confirm-row"><dt>总控备注</dt><dd>{{ form.name }}</dd></div>
              <div class="confirm-row"><dt>网站标题</dt><dd>{{ form.siteProfile.title || "未填写（保持空白）" }}</dd></div>
              <div class="confirm-row"><dt>管理员</dt><dd>{{ form.adminUsername }}</dd></div>
              <div class="confirm-row"><dt>部署隔离</dt><dd>独立数据库、独立 App、独立 Worker</dd></div>
            </dl>
            <el-alert type="info" title="提交后将进入自动部署流程。公开资料会作为该分站的初始化快照保存。" :closable="false" show-icon />
            <div class="form-actions"><el-button @click="step = 3">上一步</el-button><el-button type="primary" :loading="loading" @click="create">确认并创建</el-button></div>
          </div>
        </div>
      </section>
    </main>
    <footer class="public-footer">安全自动化开站服务</footer>
  </div>
</template>

<script>
import { checkDomain, createProvisionJob, importLogo, uploadLogo, verifyCode } from "@/api/control";

const emptyProfile = () => ({
  title: "", logoUrl: "", iconUrl: "", seoTitle: "", seoDescription: "", seoKeywords: "",
  footerCopyright: "", termsUrl: "", privacyUrl: "", homeShowcaseMode: "custom", homeShowcaseItems: [],
  friendLinks: [],
  socials: {
    email: { url: "" }, telegram: { url: "" }, x: { url: "" }, instagram: { url: "" }
  }
});

const draftStorageKey = "provision_draft";
const sessionExpiresKey = "provision_expires_at";
const readDraft = () => {
  try { return JSON.parse(sessionStorage.getItem(draftStorageKey) || "null"); }
  catch (_) { return null; }
};
const restoredProfile = (value = {}) => {
  const base = emptyProfile();
  const socials = value.socials || {};
  return {
    ...base,
    ...value,
    friendLinks: Array.isArray(value.friendLinks) ? value.friendLinks : [],
    socials: {
      email: { ...base.socials.email, ...(socials.email || {}) },
      telegram: { ...base.socials.telegram, ...(socials.telegram || {}) },
      x: { ...base.socials.x, ...(socials.x || {}) },
      instagram: { ...base.socials.instagram, ...(socials.instagram || {}) }
    }
  };
};

export default {
  name: "PublicHome",
  data() {
    return {
      step: 0,
      code: "",
      token: sessionStorage.getItem("provision_token") || "",
      loading: false,
      uploading: { logoUrl: false, iconUrl: false },
      importing: { logoUrl: false, iconUrl: false },
      assetSources: { logoUrl: "", iconUrl: "" },
      domainChecking: false,
      domainConfirmed: false,
      domainError: "",
      reservationId: "",
      reservationExpiresAt: "",
      sessionExpiresAt: sessionStorage.getItem(sessionExpiresKey) || "",
      restoringDraft: false,
      domainTimer: null,
      confirmPassword: "",
      headings: [
        { title: "验证开站资格", description: "输入管理员提供给你的完整开站卡密。" },
        { title: "设置域名与内部备注", description: "内部备注只用于总控识别，不会显示给分站客户。" },
        { title: "填写分站公开资料", description: "所有字段均可选；留空的内容不会显示，也不会回填原项目资料。" },
        { title: "创建首个管理员", description: "此账号用于登录新分站后台，请妥善保管。" },
        { title: "确认开站资料", description: "确认信息无误后，系统将创建独立分站。" }
      ],
      form: { prefix: "", name: "", siteProfile: emptyProfile(), adminUsername: "", adminPassword: "" }
    };
  },
  computed: {
    rootDomain() { return import.meta.env.VITE_ROOT_DOMAIN || "example.com"; },
    canContinueSite() { return this.domainConfirmed && !!this.reservationId && this.form.name.trim().length >= 2 && !this.domainError; },
    passwordError() { return this.confirmPassword && this.confirmPassword !== this.form.adminPassword ? "两次输入的密码不一致" : ""; },
    canContinueAdmin() {
      return this.form.adminUsername.length >= 4 && this.form.adminUsername.length <= 32 &&
        this.form.adminPassword.length >= 10 && /[A-Za-z]/.test(this.form.adminPassword) && /\d/.test(this.form.adminPassword) &&
        this.form.adminPassword === this.confirmPassword;
    }
  },
  created() { this.restoreDraft(); },
  watch: {
    step() {
      this.$nextTick(() => {
        if (this.$el) this.$el.scrollTo(0, 0);
      });
      this.persistDraft();
    },
    code() { this.persistDraft(); },
    form: { deep: true, handler() { this.persistDraft(); } }
  },
  beforeDestroy() { clearTimeout(this.domainTimer); },
  methods: {
    message(error, fallback) { return error.userMessage || fallback; },
    applyDraft(draft) {
      if (!draft) return;
      this.code = draft.code || this.code;
      this.form = {
        prefix: draft.form?.prefix || "",
        name: draft.form?.name || "",
        siteProfile: restoredProfile(draft.form?.siteProfile),
        adminUsername: draft.form?.adminUsername || "",
        adminPassword: ""
      };
      this.confirmPassword = "";
      this.reservationId = draft.reservationId || "";
      this.reservationExpiresAt = draft.reservationExpiresAt || "";
      this.domainConfirmed = !!this.reservationId && (!this.reservationExpiresAt || Date.parse(this.reservationExpiresAt) > Date.now());
      const requestedStep = Math.max(1, Math.min(Number(draft.step) || 1, 3));
      this.step = this.domainConfirmed ? requestedStep : 1;
    },
    restoreDraft() {
      const draft = readDraft();
      if (!this.token || !draft) return;
      const expiresAt = draft.sessionExpiresAt || this.sessionExpiresAt;
      if (expiresAt && Date.parse(expiresAt) <= Date.now()) {
        sessionStorage.removeItem("provision_token");
        sessionStorage.removeItem(sessionExpiresKey);
        sessionStorage.removeItem(draftStorageKey);
        this.token = "";
        return;
      }
      this.restoringDraft = true;
      this.sessionExpiresAt = expiresAt || "";
      this.applyDraft(draft);
      this.$nextTick(() => { this.restoringDraft = false; });
    },
    persistDraft() {
      if (!this.token || this.restoringDraft) return;
      const form = JSON.parse(JSON.stringify(this.form));
      form.adminPassword = "";
      sessionStorage.setItem(draftStorageKey, JSON.stringify({
        code: this.code,
        step: this.step,
        form,
        reservationId: this.reservationId,
        reservationExpiresAt: this.reservationExpiresAt,
        sessionExpiresAt: this.sessionExpiresAt
      }));
    },
    resetDraftForm() {
      this.form = { prefix: "", name: "", siteProfile: emptyProfile(), adminUsername: "", adminPassword: "" };
      this.confirmPassword = "";
      this.reservationId = "";
      this.reservationExpiresAt = "";
      this.domainConfirmed = false;
    },
    async verify() {
      if (!this.code || this.loading) return;
      this.loading = true;
      try {
        const previousDraft = readDraft();
        const sameDraft = previousDraft && String(previousDraft.code || "").trim().toUpperCase() === this.code.trim().toUpperCase();
        const result = await verifyCode(this.code);
        this.token = result.provisionToken;
        this.sessionExpiresAt = result.expiresAt || "";
        sessionStorage.setItem("provision_token", this.token);
        sessionStorage.setItem(sessionExpiresKey, this.sessionExpiresAt);
        if (result.resumeJob && result.resumeJob.id) {
          sessionStorage.removeItem(draftStorageKey);
          this.$router.push(`/progress/${result.resumeJob.id}`);
          return;
        }
        if (sameDraft) this.applyDraft(previousDraft);
        else this.resetDraftForm();
        if (result.resumeReservation && result.resumeReservation.reservationId) {
          const reservation = result.resumeReservation;
          const suffix = `.${this.rootDomain}`;
          this.reservationId = reservation.reservationId;
          this.reservationExpiresAt = reservation.expiresAt;
          this.domainConfirmed = !reservation.expiresAt || Date.parse(reservation.expiresAt) > Date.now();
          this.form.prefix = reservation.domain.endsWith(suffix) ? reservation.domain.slice(0, -suffix.length) : reservation.domain.split(".")[0];
        }
        this.step = sameDraft && this.domainConfirmed ? Math.max(1, Math.min(Number(previousDraft.step) || 1, 3)) : 1;
        this.persistDraft();
      } catch (error) { this.$message.error(this.message(error, "卡密验证失败")); }
      finally { this.loading = false; }
    },
    scheduleDomainCheck() {
      clearTimeout(this.domainTimer);
      this.domainConfirmed = false;
      this.domainError = "";
      this.reservationId = "";
      this.reservationExpiresAt = "";
      const prefix = this.form.prefix;
      if (!prefix) return;
      if (!/^[a-z0-9](?:[a-z0-9-]{1,30}[a-z0-9])?$/.test(prefix)) {
        this.domainError = "请输入 3-32 位小写字母、数字或短横线";
        return;
      }
      this.domainTimer = setTimeout(() => this.checkPrefix(prefix), 350);
    },
    async checkPrefix(prefix) {
      this.domainChecking = true;
      try {
        const result = await checkDomain(prefix, this.token);
        if (prefix === this.form.prefix) {
          this.domainConfirmed = true;
          this.reservationId = result.reservationId;
          this.reservationExpiresAt = result.expiresAt;
          if (!this.form.name) this.form.name = prefix;
          this.persistDraft();
        }
      } catch (error) {
        if (prefix === this.form.prefix) this.domainError = this.message(error, "该域名不可用");
      } finally { if (prefix === this.form.prefix) this.domainChecking = false; }
    },
    async uploadAsset(file, key) {
      if (file.size > 1024 * 1024) { this.$message.error("图片不能超过 1 MB"); return false; }
      this.uploading[key] = true;
      try {
        const result = await uploadLogo(file, this.token);
        this.form.siteProfile[key] = result.url;
      } catch (error) { this.$message.error(this.message(error, "图片上传失败")); }
      finally { this.uploading[key] = false; }
      return false;
    },
    async importAsset(key) {
      const source = this.assetSources[key];
      if (!/^https?:\/\//i.test(source) || this.importing[key]) {
        if (source) this.$message.error("请输入有效的 HTTP 或 HTTPS 图片地址");
        return;
      }
      this.importing[key] = true;
      try {
        const result = await importLogo(source, this.token);
        this.form.siteProfile[key] = result.url;
        this.assetSources[key] = "";
        this.$message.success("图片已下载并保存");
      } catch (error) { this.$message.error(this.message(error, "图片下载失败")); }
      finally { this.importing[key] = false; }
    },
    addFriendLink() { this.form.siteProfile.friendLinks.push({ label: "", url: "" }); },
    applyReservation(result) {
      this.domainConfirmed = true;
      this.domainError = "";
      this.reservationId = result.reservationId;
      this.reservationExpiresAt = result.expiresAt;
      this.persistDraft();
    },
    async renewSession() {
      if (!this.code) throw new Error("开站会话已过期，请返回第一步重新输入卡密");
      const result = await verifyCode(this.code);
      this.token = result.provisionToken;
      this.sessionExpiresAt = result.expiresAt || "";
      sessionStorage.setItem("provision_token", this.token);
      sessionStorage.setItem(sessionExpiresKey, this.sessionExpiresAt);
      if (result.resumeJob && result.resumeJob.id) {
        sessionStorage.removeItem(draftStorageKey);
        this.$router.push(`/progress/${result.resumeJob.id}`);
        return false;
      }
      return true;
    },
    async ensureReservationForCreate() {
      try {
        this.applyReservation(await checkDomain(this.form.prefix, this.token));
        return true;
      } catch (error) {
        const message = this.message(error, "");
        if (!/session|expired|会话|过期/i.test(message) || !await this.renewSession()) throw error;
        this.applyReservation(await checkDomain(this.form.prefix, this.token));
        return true;
      }
    },
    async create() {
      if (this.loading) return;
      if (!this.form.prefix || this.form.name.trim().length < 2) {
        this.$message.error("分站域名或总控备注不完整，请返回检查");
        return;
      }
      if (!this.canContinueAdmin) {
        this.$message.error("管理员账号或密码校验未通过，请返回重新填写");
        return;
      }
      this.loading = true;
      try {
        await this.ensureReservationForCreate();
        const result = await createProvisionJob({ ...this.form, reservationId: this.reservationId }, this.token);
        sessionStorage.removeItem(draftStorageKey);
        this.$router.push(`/progress/${result.job.id}`);
      } catch (error) { this.$message.error(this.message(error, "创建请求提交失败")); }
      finally { this.loading = false; }
    }
  }
};
</script>
