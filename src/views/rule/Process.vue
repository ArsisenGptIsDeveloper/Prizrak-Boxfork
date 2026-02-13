<script setup lang="ts">
import createApi from "@/api";
import {useI18n} from "vue-i18n";
import {pError, pSuccess, pWarning} from "@/util/pLoad";
import {useMenuStore} from "@/store/menuStore";

const {proxy} = getCurrentInstance()!;
const api = createApi(proxy);
const {t} = useI18n();
const menuStore = useMenuStore();

const enabled = ref(false);
const processes = ref<string[]>([]);
const processInput = ref('');

const isTunEnabled = computed(() => menuStore.tun);

const load = async () => {
  const data = await api.getProcessBypass();
  enabled.value = !!data.enable;
  processes.value = Array.isArray(data.processes) ? data.processes : [];
};

onMounted(load);

const normalize = (value: string) => value.trim();

const save = async () => {
  try {
    await api.updateProcessBypass({
      enable: enabled.value,
      processes: processes.value,
    });
    pSuccess(t('rule.success'));
  } catch (e: any) {
    pError(e?.message || 'save failed');
  }
};

const toggleEnable = async () => {
  if (!isTunEnabled.value && enabled.value) {
    pWarning(t('rule.process.warningTunOnly'));
  }
  await save();
};

const addProcess = async () => {
  const name = normalize(processInput.value);
  if (!name) {
    pWarning(t('rule.process.validation.empty'));
    return;
  }

  const exists = processes.value.some((item) => item.toLowerCase() === name.toLowerCase());
  if (exists) {
    pWarning(t('rule.process.validation.duplicate'));
    return;
  }

  processes.value = [...processes.value, name];
  processInput.value = '';
  await save();
};

const removeProcess = async (index: number) => {
  processes.value = processes.value.filter((_, i) => i !== index);
  await save();
};
</script>

<template>
  <div class="process-page">
    <el-alert
        v-if="!isTunEnabled"
        :title="$t('rule.process.warningTunOnly')"
        type="warning"
        show-icon
        :closable="false"
        class="tun-warning"
    />

    <div class="row">
      <el-text>{{ $t('on') }}</el-text>
      <el-switch v-model="enabled" @change="toggleEnable"/>
      <el-text>{{ $t('off') }}</el-text>
    </div>

    <div class="row">
      <el-input
          v-model="processInput"
          :placeholder="$t('rule.process.placeholder')"
          @keyup.enter="addProcess"
      />
      <el-button @click="addProcess">{{ $t('add') }}</el-button>
    </div>

    <div class="list">
      <div class="item" v-for="(process, index) in processes" :key="process + index">
        <span>{{ process }}</span>
        <el-button type="danger" text @click="removeProcess(index)">{{ $t('delete') }}</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.process-page {
  width: 95%;
  margin-left: 10px;
  margin-top: 10px;
}

.tun-warning {
  margin-bottom: 12px;
}

.row {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 12px;
}

.list {
  border: 1px solid var(--hr-color);
  border-radius: 8px;
  padding: 8px;
  max-width: 700px;
}

.item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--sub-card-border);
  padding: 6px 0;
}

.item:last-child {
  border-bottom: none;
}
</style>

