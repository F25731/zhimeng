<template>
  <el-container class="admin-layout">
    <el-aside width="224px" class="admin-layout__aside">
      <div class="admin-layout__brand">
        <img class="brand-logo" src="/logo.svg" alt="" />
        <span>分站开通</span>
      </div>
      <el-menu router :default-active="$route.path">
        <el-menu-item v-for="item in navigation" :key="item.path" :index="item.path">
          <i :class="item.icon" />
          <span>{{ item.label }}</span>
        </el-menu-item>
      </el-menu>
      <div class="admin-layout__aside-footer">CONTROL CENTER</div>
    </el-aside>
    <el-container class="admin-layout__main">
      <el-header class="admin-layout__header">
        <div class="header-title">{{ $route.meta.title || "分站开通" }}</div>
        <div class="header-actions">
          <el-tooltip :content="theme === 'dark' ? '切换白色模式' : '切换黑色模式'" placement="bottom">
            <el-button :icon="theme === 'dark' ? 'el-icon-sunny' : 'el-icon-moon'" circle size="small" :aria-label="theme === 'dark' ? '切换白色模式' : '切换黑色模式'" @click="toggleTheme" />
          </el-tooltip>
          <el-tooltip content="刷新当前页面" placement="bottom">
            <el-button icon="el-icon-refresh" circle size="small" @click="refresh" />
          </el-tooltip>
          <span class="admin-user">{{ admin && admin.username }}</span>
          <el-dropdown trigger="click" @command="handleCommand">
            <el-button icon="el-icon-user" circle size="small" aria-label="账户菜单" />
            <el-dropdown-menu slot="dropdown">
              <el-dropdown-item disabled>{{ admin && admin.role === "super_admin" ? "超级管理员" : "管理员" }}</el-dropdown-item>
              <el-dropdown-item divided command="logout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="admin-content">
        <div class="content-wrap"><router-view :key="viewKey" /></div>
      </el-main>
    </el-container>
  </el-container>
</template>

<script>
import { logout, me } from "@/api/auth";
import { applyTheme, getTheme } from "@/utils/theme";

export default {
  name: "AdminLayout",
  data() {
    return {
      refreshCounter: 0,
      theme: getTheme(),
      navigation: [
        { path: "/zhimeng/dashboard", label: "系统概览", icon: "el-icon-data-analysis" },
        { path: "/zhimeng/cards", label: "开站卡密", icon: "el-icon-tickets" },
        { path: "/zhimeng/sites", label: "分站管理", icon: "el-icon-monitor" },
        { path: "/zhimeng/jobs", label: "任务中心", icon: "el-icon-s-operation" },
        { path: "/zhimeng/versions", label: "版本管理", icon: "el-icon-collection-tag" },
        { path: "/zhimeng/nodes", label: "部署节点", icon: "el-icon-cpu" },
        { path: "/zhimeng/audit", label: "操作日志", icon: "el-icon-document" }
      ]
    };
  },
  computed: {
    admin() { return this.$store.state.admin; },
    viewKey() { return `${this.$route.fullPath}:${this.refreshCounter}`; }
  },
  async created() {
    try {
      const data = await me();
      this.$store.commit("setAdmin", data.admin);
      this.$store.commit("setCSRFToken", data.csrfToken);
    } catch (_) {
      this.$router.replace({ path: "/zhimeng/login", query: { redirect: this.$route.fullPath } });
    }
  },
  methods: {
    toggleTheme() { this.theme = applyTheme(this.theme === "dark" ? "light" : "dark"); },
    refresh() { this.refreshCounter += 1; },
    async handleCommand(command) {
      if (command !== "logout") return;
      try { await logout(); } finally {
        this.$store.commit("setAdmin", null);
        this.$store.commit("setCSRFToken", "");
        this.$router.replace("/zhimeng/login");
      }
    }
  }
};
</script>
