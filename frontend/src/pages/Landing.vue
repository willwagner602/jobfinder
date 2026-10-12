<script setup lang="ts">
import { RouterLink } from 'vue-router';
import { session } from '../api';
</script>
<template><section><div class="hero"><div><p class="pill">Welcome</p><h1 class="text-4xl">Welcome to jobs.willwagner.me.</h1><p class="text-muted">A section for information about the service will live here.</p></div><div class="card info-placeholder" data-testid="info-section"><h2 class="text-lg">Information</h2><p class="text-muted">Content for this section has not been added yet.</p></div></div><div class="actions mt-6" data-testid="landing-actions"><RouterLink v-if="!session.authenticated" class="button" to="/signup">Create an account</RouterLink><RouterLink v-else class="button" to="/dashboard">Go to dashboard</RouterLink><RouterLink v-if="!session.authenticated" class="button secondary" to="/login">Log in</RouterLink></div></section></template>
