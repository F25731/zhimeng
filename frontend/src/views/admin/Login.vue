<template>
  <main class="login-shell">
    <section class="login-panel">
      <div class="login-panel__brand"><img class="brand-logo" src="/logo.svg" alt="" /><span>分站开通</span></div>
      <el-form class="login-form" @submit.native.prevent="submit">
        <h1>登录总控后台</h1>
        <p>使用管理员账号管理开站、分站运行与版本升级。</p>
        <el-form-item label="用户名">
          <el-input v-model.trim="username" prefix-icon="el-icon-user" autocomplete="username" autofocus @keyup.enter.native="submit" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="password" type="password" prefix-icon="el-icon-lock" show-password autocomplete="current-password" @keyup.enter.native="submit" />
        </el-form-item>
        <el-button type="primary" native-type="submit" :loading="submitting">登录</el-button>
      </el-form>
    </section>
    <section class="login-context">
      <div class="login-context__content">
        <h2>统一管理每一个独立分站</h2>
        <p>开站任务、独立数据库、容器状态、运营快照和版本升级集中在一个后台处理。</p>
      </div>
    </section>
  </main>
</template>

<script>
import { login } from "@/api/auth";

export default {
  name: "AdminLogin",
  data() { return { username: "", password: "", submitting: false }; },
  methods: {
    async submit() {
      if (!this.username || !this.password || this.submitting) {
        if (!this.submitting) this.$message.warning("请输入用户名和密码");
        return;
      }
      this.submitting = true;
      try {
        const data = await login({ username: this.username, password: this.password });
        this.$store.commit("setAdmin", data.admin);
        this.$store.commit("setCSRFToken", data.csrfToken);
        this.$router.replace(this.$route.query.redirect || "/zhimeng/dashboard");
      } catch (error) { this.$message.error(error.userMessage || "登录失败"); }
      finally { this.submitting = false; }
    }
  }
};
</script>
