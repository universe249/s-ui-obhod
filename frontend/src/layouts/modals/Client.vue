<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="800"
  >
    <v-card
      class="rounded-lg"
      :loading="loading"
    >
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.client') }}
      </v-card-title>
      <v-divider />
      <v-skeleton-loader
        v-if="loading"
        class="mx-auto border"
        width="95%"
        type="card, text, divider, list-item-two-line"
      />
      <v-card-text style="padding: 0 16px; overflow-y: scroll;">
        <v-container
          style="padding: 0;"
          :hidden="loading"
        >
          <v-tabs
            v-model="tab"
            align-tabs="center"
          >
            <v-tab value="t1">
              {{ $t('client.basics') }}
            </v-tab>
            <v-tab value="t2">
              {{ $t('client.config') }}
            </v-tab>
            <v-tab value="t3">
              {{ $t('client.links') }}
            </v-tab>
            <v-tab
              v-if="id > 0"
              value="t4"
            >
              {{ $t('client.devices') }}<span v-if="(client.deviceLimit ?? 0) > 0">&nbsp;{{ devices.length }}/{{ client.deviceLimit }}</span>
            </v-tab>
          </v-tabs>
          <v-window v-model="tab">
            <v-window-item value="t1">
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-switch
                    v-model="client.enable"
                    color="primary"
                    :label="$t('enable')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-combobox
                    v-model="client.group"
                    :items="groups"
                    :label="$t('client.group')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    v-model="client.name"
                    :label="$t('client.name')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    v-model="client.desc"
                    :label="$t('client.desc')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    v-model="client.remark"
                    :label="$t('client.remark')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    v-model.number="Volume"
                    type="number"
                    min="0"
                    :label="$t('stats.volume')"
                    suffix="GiB"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    v-model.number="deviceLimit"
                    type="number"
                    min="0"
                    max="100"
                    :label="$t('client.deviceLimit')"
                    :hint="graceText || $t('client.deviceLimitHint')"
                    persistent-hint
                  />
                </v-col>
                <v-col
                  v-if="!(client.delayStart && !client.autoReset)"
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <DatePick
                    :expiry="expDate"
                    @submit="setDate"
                  />
                </v-col>
                <v-col
                  v-if="client.autoReset || client.delayStart"
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    v-model.number="resetDays"
                    type="number"
                    min="1"
                    :label="$t('client.resetDays')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-switch
                    v-model="delayStart"
                    color="primary"
                    :disabled="client.up+client.down>0"
                    :label="$t('client.delayStart')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-switch
                    v-model="autoReset"
                    color="primary"
                    :label="$t('client.autoReset')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-row v-if="id > 0">
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                  class="d-flex flex-column"
                >
                  <div class="d-flex justify-space-between align-center">
                    <div>
                      {{ $t('stats.usage') }}: {{ total }}<sup
                        v-if="percent>0"
                        dir="ltr"
                      >({{ percent }}%)</sup>
                    </div>
                    <v-btn
                      :aria-label="$t('reset')"
                      density="compact"
                      variant="text"
                      icon="mdi-restore"
                      @click="resetUsage"
                    >
                      <v-tooltip
                        activator="parent"
                        location="top"
                      >
                        {{ $t('reset') }}
                      </v-tooltip>
                      <v-icon />
                    </v-btn>
                  </div>
                  <v-progress-linear
                    v-if="client.volume>0"
                    v-model="percent"
                    :color="percentColor"
                    bottom
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-icon
                    icon="mdi-upload"
                    color="warning"
                  /><span class="text-warning">{{ up }}</span>
                  / 
                  <v-icon
                    icon="mdi-download"
                    color="success"
                  /><span class="text-success">{{ down }}</span>
                </v-col>
              </v-row>
              <v-row v-if="id >0 && client.autoReset">
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <div class="text-medium-emphasis">
                    {{ $t('client.nextReset') }}
                  </div>
                  <div dir="ltr">
                    {{ nextResetFormatted }}
                  </div>
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <div class="text-medium-emphasis">
                    {{ $t('main.stats.totalUsage') }}
                  </div>
                  <div>
                    <v-icon
                      icon="mdi-upload"
                      color="warning"
                    /><span class="text-warning">{{ totalUp }}</span>
                    /
                    <v-icon
                      icon="mdi-download"
                      color="success"
                    /><span class="text-success">{{ totalDown }}</span>
                  </div>
                </v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-select
                    v-model="clientInbounds"
                    :items="inboundTags"
                    :label="$t('client.inboundTags')"
                    clearable
                    multiple
                    chips
                    hide-details
                  >
                    <template #append>
                      <v-icon
                        v-tooltip:top="$t('all')"
                        :aria-label="$t('all')"
                        icon="mdi-set-all"
                        @click="setAllInbounds"
                      />
                    </template>
                  </v-select>
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="t2">
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-btn
                    variant="tonal"
                    @click="shuffle()"
                  >
                    {{ $t('reset') + ' - ' + $t('all') }}<v-icon icon="mdi-refresh" />
                  </v-btn>
                </v-col>
              </v-row>
              <v-row
                v-for="key in Object.keys(clientConfig)"
                :key="key"
              >
                <v-col
                  cols="12"
                  md="3"
                  align="end"
                  align-self="center"
                >
                  {{ key }}
                  <v-icon
                    v-tooltip:top="$t('reset')"
                    :aria-label="$t('reset')"
                    icon="mdi-refresh"
                    @click="shuffle(key)"
                  />
                </v-col>
                <v-col>
                  <v-text-field
                    v-if="clientConfig[key].password != undefined"
                    v-model="clientConfig[key].password"
                    label="Password"
                    hide-details
                  />
                  <v-text-field
                    v-if="clientConfig[key].uuid != undefined"
                    v-model="clientConfig[key].uuid"
                    label="UUID"
                    hide-details
                  />
                  <v-text-field
                    v-if="key == 'vless'"
                    v-model="clientConfig[key].flow"
                    label="Flow"
                    hide-details
                  />
                  <v-text-field
                    v-if="key == 'hysteria'"
                    v-model="clientConfig[key].auth_str"
                    label="Auth"
                    hide-details
                  />
                  <v-text-field
                    v-if="key == 'snell'"
                    v-model="clientConfig[key].userkey"
                    label="User Key"
                    hide-details
                  />
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="t4">
              <v-row v-if="devices.length == 0">
                <v-col class="text-medium-emphasis">
                  {{ $t('client.noDevices') }}
                </v-col>
              </v-row>
              <v-row
                v-for="d in devices"
                :key="d.id"
                align="center"
                class="ob-dev"
              >
                <v-col>
                  <div>
                    <b>{{ d.model || d.os || d.userAgent || ('#' + d.id) }}</b>
                    <v-chip
                      v-if="isOnline(d)"
                      size="x-small"
                      color="success"
                      class="ms-2"
                    >
                      {{ $t('client.online') }}
                    </v-chip>
                  </div>
                  <div class="text-caption text-medium-emphasis">
                    {{ [d.os, d.userAgent].filter(x => x).join(' · ') }}
                  </div>
                  <div class="text-caption text-medium-emphasis">
                    {{ $t('client.added') }}: {{ fmtTime(d.createdAt) }} · {{ $t('client.lastSeen') }}: {{ fmtTime(d.lastSeen) }}<span v-if="d.lastIp"> · {{ d.lastIp }}</span>
                  </div>
                  <div class="text-caption text-medium-emphasis">
                    {{ $t('client.traffic') }}: ↑{{ size(d.up) }} ↓{{ size(d.down) }}<span v-if="d.sessions > 0"> · {{ d.sessions }} {{ $t('client.conns') }} · {{ $t('client.ips') }}: {{ d.ips.join(', ') }}</span>
                  </div>
                </v-col>
                <v-col cols="auto">
                  <v-btn
                    v-tooltip:top="$t('actions.deviceDel')"
                    icon="mdi-delete"
                    variant="text"
                    color="error"
                    :aria-label="$t('actions.deviceDel')"
                    @click="delDevice(d.id)"
                  />
                </v-col>
              </v-row>
              <v-row v-if="devices.length > 0">
                <v-col>
                  <v-btn
                    variant="tonal"
                    prepend-icon="mdi-key-change"
                    @click="rotateDevices"
                  >
                    {{ $t('client.rotate') }}
                  </v-btn>
                </v-col>
              </v-row>
              <v-row v-if="events.length > 0">
                <v-col>
                  <div class="text-subtitle-2 mb-1">
                    {{ $t('client.events') }}
                  </div>
                  <div
                    v-for="e in events"
                    :key="e.id"
                    class="text-caption"
                  >
                    <v-icon
                      size="x-small"
                      :color="evColor(e.kind)"
                      :icon="evIcon(e.kind)"
                    />
                    {{ fmtTime(e.at) }} — {{ $t('client.ev.' + e.kind) }}<span v-if="e.model || e.userAgent">: {{ e.model || e.userAgent }}</span><span v-if="e.ip"> ({{ e.ip }})</span><span v-if="e.actor && e.actor != 'auto'"> · {{ e.actor }}</span>
                  </div>
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="t3">
              <v-row
                v-for="(lnk, index) in links"
                :key="index"
              >
                <v-col cols="auto">
                  {{ index + 1 }}
                </v-col>
                <v-col style="direction: ltr; overflow-y: hidden;">
                  {{ lnk.uri }}
                </v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-btn
                    color="primary"
                    @click="extLinks.push({ type: 'external', uri: ''})"
                  >
                    {{ $t('actions.add') }} {{ $t('client.external') }}
                  </v-btn>
                </v-col>
              </v-row>
              <v-row
                v-for="(lnk, index) in extLinks"
                :key="index"
              >
                <v-col>
                  <v-text-field
                    v-model="lnk.uri"
                    dir="ltr"
                    :label="$t('client.external') + ' ' + (index+1)"
                    append-icon="mdi-delete"
                    placeholder="<protocol>://<data>"
                    @click:append="extLinks.splice(index,1)"
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-btn
                    color="primary"
                    @click="subLinks.push({ type: 'sub', uri: ''})"
                  >
                    {{ $t('actions.add') }} {{ $t('client.sub') }}
                  </v-btn>
                </v-col>
              </v-row>
              <v-row
                v-for="(lnk, index) in subLinks"
                :key="index"
              >
                <v-col>
                  <v-text-field
                    v-model="lnk.uri"
                    dir="ltr"
                    :label="$t('client.sub') + ' ' + (index+1)"
                    append-icon="mdi-delete"
                    placeholder="http[s]://<domain>[:]<port>/<path>"
                    @click:append="subLinks.splice(index,1)"
                  />
                </v-col>
              </v-row>
            </v-window-item>
          </v-window>
        </v-container>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { createClient, randomConfigs, updateConfigs, Link, shuffleConfigs, Device, DeviceEvent } from '@/types/clients'
import HttpUtils from '@/plugins/httputil'
import DatePick from '@/components/DateTime.vue'
import { HumanReadable } from '@/plugins/utils'
import Data from '@/store/modules/data'
import { locale } from '@/locales'
import type { PropType } from 'vue'

// The per-protocol credentials of a client. The type is internal to
// @/types/clients, so it is taken from what builds one.
type ClientConfig = ReturnType<typeof randomConfigs>

// An inbound the client can be attached to, as the parent lists them.
interface InboundTag {
  title: string
  value: number
}

export default {
  components: { DatePick },
  props: {
    visible: { type: Boolean, required: true },
    id: { type: Number, required: true },
    inboundTags: { type: Array as PropType<InboundTag[]>, required: true },
    groups: { type: Array as PropType<string[]>, required: true },
  },
  emits: ['close'],
  data() {
    return {
      client: createClient(),
      title: "add",
      loading: false,
      tab: "t1",
      clientConfig: <ClientConfig>{},
      links: <Link[]>[],
      extLinks: <Link[]>[],
      subLinks: <Link[]>[],
      devices: <Device[]>[],
      events: <DeviceEvent[]>[],
    }
  },
  computed: {
    clientInbounds: {
      get() { return this.client.inbounds.length>0 ? [...this.client.inbounds].sort() : [] },
      set(v:number[]) { this.client.inbounds = v.length == 0 ?  [] : v.sort() }
    },
    expDate: {
      get() { return this.client.expiry},
      set(v:number) { this.client.expiry = v }
    },
    Volume: {
      get() { return this.client.volume == 0 ? 0 : (this.client.volume / (1024 ** 3)) },
      set(v:number) { this.client.volume = v > 0 ? v*(1024 ** 3) : 0 }
    },
    delayStart: {
      get() { return this.client.delayStart?? false },
      set(v:boolean) {
        this.client.delayStart = v
        this.client.resetDays = v ? 1 : 0
        if (v && !this.autoReset) this.client.expiry = 0
      }
    },
    autoReset: {
      get() { return this.client.autoReset?? false },
      set(v:boolean) {
        this.client.autoReset = v
        this.client.resetDays = v ? 1 : 0
        if (!v) this.client.nextReset = 0
      }
    },
    resetDays: {
      get() { return this.client.resetDays?? 1 },
      set(v:number|null) {
        if (!v) v = 1
        if (this.client.nextReset && this.client.nextReset > 0) {
          this.client.nextReset += (v-(this.client.resetDays?? 0))*24*60*60
        }
        this.client.resetDays = v
      }
    },
    graceText(): string {
      const g = this.client.deviceGraceUntil ?? 0
      if ((this.client.deviceLimit ?? 0) <= 0 || g * 1000 <= Date.now()) return ''
      const fmt = (t:number) => new Date(t * 1000).toLocaleString()
      const seen = this.client.legacySeenAt ?? 0
      return this.$t('client.graceUntil', { date: fmt(g) }) + ' · ' +
        (seen ? this.$t('client.legacySeen', { date: fmt(seen) }) : this.$t('client.legacyNone'))
    },
    deviceLimit: {
      get() { return this.client.deviceLimit ?? 0 },
      set(v:number|null) { this.client.deviceLimit = v && v > 0 ? Math.min(v, 100) : 0 }
    },
    up() :string { return HumanReadable.sizeFormat(this.client.up) },
    down() :string { return HumanReadable.sizeFormat(this.client.down) },
    total() :string { return HumanReadable.sizeFormat(this.client.down + this.client.up) },
    totalUp() :string { return HumanReadable.sizeFormat((this.client.totalUp ?? 0) + this.client.up) },
    totalDown() :string { return HumanReadable.sizeFormat((this.client.totalDown ?? 0) + this.client.down) },
    nextResetFormatted() :string {
      const ts = this.client.nextReset?? 0
      if (ts == 0) return '-'
      const date = new Date(ts*1000)
      return date.toLocaleString(locale)
    },
    percent() :number { return this.client.volume>0 ? Math.round((this.client.up + this.client.down) *100 / this.client.volume) : 0 },
    percentColor() :string { return (this.client.up+this.client.down) >= this.client.volume ? 'error' : this.percent>90 ? 'warning' : 'success' },
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.updateData(this.$props.id)
      }
    },
  },
  methods: {
    async updateData(id: number) {
      if (id > 0) {
        this.loading = true
        const newData = await Data().loadClients(id)
        this.client = createClient(newData)
        this.title = "edit"
        this.clientConfig = <ClientConfig>this.client.config
        this.loading = false
      }
      else {
        this.client = createClient()
        this.title = "add"
        this.clientConfig = randomConfigs('client')
      }
      this.links = this.client.links?.filter(l => l.type == 'local')?? []
      this.extLinks = this.client.links?.filter(l => l.type == 'external')?? []
      this.subLinks = this.client.links?.filter(l => l.type == 'sub')?? []
      this.devices = []
      this.events = []
      if (id > 0) this.loadDevices()
      this.tab = "t1"
      this.loading = false
    },
    closeModal() {
      this.updateData(0) // reset
      this.$emit('close')
    },
    async saveChanges() {
      if (!this.$props.visible) return
      // check duplicate name
      const isDuplicateName = Data().checkClientName(this.$props.id, this.client.name)
      if (isDuplicateName) return

      // check if delayStart is true and autoReset is false, set expiry to 0
      if (this.client.delayStart && !this.client.autoReset) this.client.expiry = 0

      // save data
      this.loading = true
      this.client.config = updateConfigs(this.clientConfig, this.client.name)
      this.client.links = [
                        ...this.extLinks.filter(l => l.uri != ''),
                        ...this.subLinks.filter(l => l.uri != '')]
      const success = await Data().save("clients", this.$props.id == 0 ? "new" : "edit", this.client)
      if (success) this.closeModal()
      this.loading = false
    },
    setDate(newDate:number){
      this.client.expiry = newDate
    },
    setAllInbounds(){
      this.client.inbounds = this.inboundTags.map(i => i.value).sort()
    },
    shuffle(k?:string) {
      shuffleConfigs(this.clientConfig, k)
    },
    async loadDevices() {
      const msg = await HttpUtils.get<Device[]>('api/devices', { c: this.$props.id })
      this.devices = msg.success && msg.obj ? msg.obj : []
      const ev = await HttpUtils.get<DeviceEvent[]>('api/deviceEvents', { c: this.$props.id, n: 20 })
      this.events = ev.success && ev.obj ? ev.obj : []
    },
    size(b: number): string { return HumanReadable.sizeFormat(b ?? 0) },
    evIcon(k: string): string {
      return ({ added: 'mdi-plus-circle', deleted: 'mdi-delete', rotated: 'mdi-key-change', limit: 'mdi-alert', nohwid: 'mdi-help-circle' } as Record<string, string>)[k] ?? 'mdi-circle-small'
    },
    evColor(k: string): string {
      return ({ added: 'success', deleted: 'grey', rotated: 'info', limit: 'error', nohwid: 'warning' } as Record<string, string>)[k] ?? ''
    },
    async delDevice(id: number) {
      const msg = await HttpUtils.post('api/deviceDel', { ids: String(id) })
      if (msg.success) this.loadDevices()
    },
    async rotateDevices() {
      const msg = await HttpUtils.post('api/deviceRotate', { c: this.$props.id, grace: 24 })
      if (msg.success) this.loadDevices()
    },
    isOnline(d: Device): boolean { return d.onlineAt > 0 && Date.now()/1000 - d.onlineAt < 60 },
    fmtTime(ts: number): string { return ts > 0 ? new Date(ts*1000).toLocaleString(locale) : '-' },
    resetUsage(){
      this.client.totalUp = (this.client.totalUp ?? 0) + this.client.up
      this.client.totalDown = (this.client.totalDown ?? 0) + this.client.down
      this.client.up = 0
      this.client.down = 0
    }
  },
}

</script>