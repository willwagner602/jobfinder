<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RouterLink } from 'vue-router';
import { apiGet, errorMessage, Job } from '../api';
const loading = ref(true), error = ref(''), jobs = ref<Job[]>([]);
onMounted(async()=>{ try{ const data=await apiGet<{jobs:Job[]}>('/api/jobs'); jobs.value=data.jobs || []; } catch(err){ error.value=errorMessage(err);} finally{ loading.value=false; } });
</script>
<template><section><div class="mb-4"><p class="pill">Job matches</p><h1 class="text-4xl">Review your matches</h1><p class="text-muted">Up to 100 newest matches from the server, with decisions and ratings when available.</p></div><p v-if="loading" class="card" role="status">Loading job matches…</p><p v-else-if="error" class="alert error" role="alert">{{ error }}</p><div v-else-if="jobs.length" class="grid gap-4"><RouterLink v-for="job in jobs" :key="job.matchId" class="card job-card" :to="`/jobs/${job.matchId}`" :aria-label="`View ${job.title} at ${job.company}`"><span class="pill">{{ job.decision }}</span><h2 class="text-2xl">{{ job.title }}</h2><p class="meta"><span>{{ job.company }}</span><span v-if="job.rating">Rating {{ job.rating }}/5</span><span v-if="job.createdAt">{{ new Date(job.createdAt).toLocaleDateString() }}</span></p><p v-if="job.summary" class="text-muted">{{ job.summary }}</p></RouterLink></div><div v-else class="empty"><h2>No matches yet</h2><p>Save your profile to request a prior-week match run.</p><RouterLink class="button mt-4" to="/onboarding">Update profile</RouterLink></div></section></template>
