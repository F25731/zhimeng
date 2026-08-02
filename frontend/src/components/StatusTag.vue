<template>
  <el-tag :type="type" size="small" effect="plain">{{ label }}</el-tag>
</template>

<script>
import { statusLabels, statusTypes } from "@/utils/format";

export default {
  name: "StatusTag",
  props: { status: { type: String, default: "" } },
  computed: {
    label() { return statusLabels[this.status] || this.status || "未知"; },
    type() { return statusTypes[this.status] || "info"; }
  }
};
</script>
