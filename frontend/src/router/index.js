import Vue from "vue";
import Router from "vue-router";

import AdminLayout from "@/layouts/AdminLayout.vue";

const Home = () => import("@/views/public/Home.vue");
const Progress = () => import("@/views/public/Progress.vue");
const Login = () => import("@/views/admin/Login.vue");
const Dashboard = () => import("@/views/admin/Dashboard.vue");
const Cards = () => import("@/views/admin/Cards.vue");
const Sites = () => import("@/views/admin/Sites.vue");
const SiteDetail = () => import("@/views/admin/SiteDetail.vue");
const Jobs = () => import("@/views/admin/Jobs.vue");
const Versions = () => import("@/views/admin/Versions.vue");
const Nodes = () => import("@/views/admin/Nodes.vue");
const Audit = () => import("@/views/admin/Audit.vue");

Vue.use(Router);

export default new Router({
  mode: "history",
  routes: [
    { path: "/", name: "home", component: Home },
    { path: "/progress/:jobId", name: "progress", component: Progress },
    { path: "/zhimeng/login", name: "admin-login", component: Login },
    {
      path: "/zhimeng",
      component: AdminLayout,
      children: [
        { path: "", redirect: "dashboard" },
        { path: "dashboard", name: "admin-dashboard", component: Dashboard, meta: { title: "系统概览" } },
        { path: "cards", name: "admin-cards", component: Cards, meta: { title: "开站卡密" } },
        { path: "sites", name: "admin-sites", component: Sites, meta: { title: "分站管理" } },
        { path: "sites/:id", name: "admin-site-detail", component: SiteDetail, meta: { title: "分站详情" } },
        { path: "jobs", name: "admin-jobs", component: Jobs, meta: { title: "任务中心" } },
        { path: "versions", name: "admin-versions", component: Versions, meta: { title: "版本管理" } },
        { path: "nodes", name: "admin-nodes", component: Nodes, meta: { title: "部署节点" } },
        { path: "audit", name: "admin-audit", component: Audit, meta: { title: "操作日志" } }
      ]
    }
  ]
});
