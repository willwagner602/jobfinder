<script setup lang="ts">
import { RouterLink } from 'vue-router';
import { session } from '../api';
</script>
<template><section class="page-stack"><header class="page-intro"><p class="eyebrow">Welcome</p><h1>Welcome to jobs.willwagner.me.</h1><p class="muted">A section for information about the service will live here.</p></header><div class="card info-placeholder" data-testid="info-section"><h2>Information</h2><p class="muted">Content for this section has not been added yet.</p></div><div class="actions actions-start" data-testid="landing-actions"><RouterLink v-if="!session.authenticated" class="button" to="/signup">Create an account</RouterLink><RouterLink v-else class="button" to="/dashboard">Go to dashboard</RouterLink><RouterLink v-if="!session.authenticated" class="button secondary" to="/login">Log in</RouterLink></div></section></template>
