<script setup lang="ts">
import Header from "./components/Header.vue";
import { computed } from "vue";
import { RouterView, useRoute, useRouter } from "vue-router";
import { userStore } from "@/stores/user";

const route = useRoute();
const router = useRouter();
const { info, isLogin } = userStore();
const routeViewKey = computed(() => route.path);
const showDefaultPasswordWarning = computed(
  () => isLogin.value && info.value?.mustChangePassword === true
);

const openPasswordSettings = () => {
  router.push({
    path: "/user/me",
    query: { changePassword: "1" }
  });
};
</script>

<template>
  <Header />
  <section
    v-if="showDefaultPasswordWarning"
    class="mx-auto mt-3 flex max-w-7xl flex-col gap-3 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-amber-950 shadow-sm dark:border-amber-700 dark:bg-amber-950 dark:text-amber-100 sm:flex-row sm:items-center sm:justify-between"
    role="alert"
    aria-live="polite"
  >
    <div>
      <strong class="block">安全提醒：root 仍在使用初始默认密码</strong>
      <span class="text-sm">
        建议尽快改成至少 8 位的新密码。在修改前，播放和管理功能仍可照常使用。
      </span>
    </div>
    <button
      type="button"
      class="btn btn-warning m-0 shrink-0 self-start sm:self-auto"
      @click="openPasswordSettings"
    >
      立即修改密码
    </button>
  </section>
  <el-container>
    <el-main>
      <RouterView :key="routeViewKey" />
    </el-main>
  </el-container>
</template>
