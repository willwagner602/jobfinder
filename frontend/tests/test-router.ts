import { createRouter, createMemoryHistory } from 'vue-router';
import type { RouteRecordRaw } from 'vue-router';
import Dashboard from '../src/pages/Dashboard.vue';
import Signup from '../src/pages/Signup.vue';
import Verify from '../src/pages/Verify.vue';
import Login from '../src/pages/Login.vue';
import Landing from '../src/pages/Landing.vue';
import Profile from '../src/pages/Profile.vue';
import JobsList from '../src/pages/JobsList.vue';
import JobDetail from '../src/pages/JobDetail.vue';

export function createTestRouter(initialPath = '/') {
  const routes: RouteRecordRaw[] = [
    { path: '/', component: Landing },
    { path: '/dashboard', component: Dashboard },
    { path: '/signup', component: Signup },
    { path: '/verify', component: Verify },
    { path: '/login', component: Login },
    { path: '/onboarding', component: Profile },
    { path: '/profile', redirect: '/onboarding' },
    { path: '/jobs', component: JobsList },
    { path: '/jobs/:matchId', component: JobDetail },
  ];
  const router = createRouter({ history: createMemoryHistory(), routes });
  router.push(initialPath);
  return router;
}
