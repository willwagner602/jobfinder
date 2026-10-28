<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RouterLink } from 'vue-router';
import { apiGet, errorMessage, Job, Profile, session } from '../api';
const loading = ref(true), error = ref('');
const profile = ref<Profile>({}), recentJobs = ref<Job[]>([]);
onMounted(async () => {
  try {
    const data = await apiGet<{ profile: Profile; recentJobs: Job[] }>('/api/dashboard');
    profile.value = data.profile || {};
    recentJobs.value = data.recentJobs || [];
  } catch (err) { error.value = errorMessage(err); }
  finally { loading.value = false; }
});
</script>
<template>
  <section class="page-stack">
    <header class="page-intro">
      <p class="eyebrow">Dashboard</p>
      <h1>Welcome<span v-if="session.user">, {{ session.user.email }}</span></h1>
      <p class="muted">Keep your profile current, then review the newest job matches.</p>
    </header>
    <section class="card dashboard-next" aria-labelledby="next-steps-title">
      <h2 id="next-steps-title">Next steps</h2>
      <div class="actions actions-start">
        <RouterLink class="button" to="/onboarding">Update resume and experience</RouterLink>
        <RouterLink class="button secondary" to="/jobs">View job matches</RouterLink>
      </div>
    </section>
    <p v-if="loading" class="card" role="status">Loading dashboard…</p>
    <p v-else-if="error" class="alert error" role="alert">{{ error }}</p>
    <div v-else class="dashboard-grid">
      <article class="card content-card"><h2>Profile status</h2><p class="muted">{{ profile.hasProfile ? `Profile saved${profile.documentCount ? ` · ${profile.documentCount} document${profile.documentCount === 1 ? '' : 's'} uploaded` : ''}.` : 'Profile not set up yet. Upload documents and choose desired titles and blocked companies.' }}</p><p v-if="profile.lastMatchRunMessage" class="alert success" role="status">{{ profile.lastMatchRunMessage }}</p></article>
      <article class="card content-card"><h2>Recent matches</h2><div v-if="recentJobs.length" class="recent-list"><RouterLink v-for="job in recentJobs" :key="job.matchId" class="recent-job" :to="`/jobs/${job.matchId}`"><strong>{{ job.title }}</strong><span class="muted">{{ job.company }}</span><span class="job-status">{{ job.decision }}<template v-if="job.rating"> · {{ job.rating }}/5</template></span></RouterLink></div><div v-else class="empty">No matches yet. Save your profile to request a prior-week match run.</div></article>
    </div>
  </section>
</template>
