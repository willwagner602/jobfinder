<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RouterLink } from 'vue-router';
import { apiGet, errorMessage, Job } from '../api';
const loading = ref(true), error = ref(''), jobs = ref<Job[]>([]);
onMounted(async () => { try { const data = await apiGet<{ jobs: Job[] }>('/api/jobs'); jobs.value = data.jobs || []; } catch (err) { error.value = errorMessage(err); } finally { loading.value = false; } });
</script>
<template>
  <section class="page-stack">
    <header class="page-intro"><p class="eyebrow">Job matches</p><h1>Review your matches</h1><p class="muted">Up to 100 newest matches from the server, with decisions and ratings when available.</p></header>
    <p v-if="loading" class="card" role="status">Loading job matches…</p><p v-else-if="error" class="alert error" role="alert">{{ error }}</p>
    <div v-else-if="jobs.length" class="job-list"><RouterLink v-for="job in jobs" :key="job.matchId" class="card job-card" :to="`/jobs/${job.matchId}`" :aria-label="`View ${job.title} at ${job.company}`"><span class="pill job-decision">{{ job.decision }}</span><h2>{{ job.title }}</h2><p class="meta"><span>{{ job.company }}</span><span v-if="job.rating">Rating {{ job.rating }}/5</span><span v-if="job.createdAt">{{ new Date(job.createdAt).toLocaleDateString() }}</span></p><p v-if="job.summary" class="muted job-summary">{{ job.summary }}</p></RouterLink></div>
    <div v-else class="empty"><h2>No matches yet</h2><p>Save your profile to request a prior-week match run.</p><RouterLink class="button mt-4" to="/onboarding">Update profile</RouterLink></div>
  </section>
</template>
