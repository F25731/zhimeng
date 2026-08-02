<template><div><PageHeader title="任务中心" description="查看开站、升级和运维任务，处理失败重试" /><JobsTable /></div></template>
<script>
import PageHeader from "@/components/PageHeader.vue";
import JobsTable from "@/views/admin/components/JobsTable.vue";
export default { name: "JobsPage", components: { PageHeader, JobsTable } };
</script>
