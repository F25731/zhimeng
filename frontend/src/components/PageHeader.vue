<template>
  <header class="page-header">
    <div>
      <h1>{{ title }}</h1>
      <p v-if="description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="page-header__actions"><slot name="actions" /></div>
  </header>
</template>

<script>
export default {
  name: "PageHeader",
  props: { title: { type: String, required: true }, description: { type: String, default: "" } }
};
</script>
