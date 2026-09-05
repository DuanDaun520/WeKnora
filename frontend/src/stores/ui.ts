import { defineStore } from 'pinia'

export const useUIStore = defineStore('ui', {
  state: () => ({
    showSettingsModal: false,
    showKBEditorModal: false,
    kbEditorMode: 'create' as 'create' | 'edit',
    currentKBId: null as string | null,
    kbEditorType: 'document' as 'document' | 'faq',
    // 当前选中的标签 ID，用于文件上传时传递
    selectedTagIds: [] as string[],
    kbEditorInitialSection: null as string | null,
    settingsInitialSection: null as string | null,
    settingsInitialSubSection: null as string | null,
    // 000101 空间管理员成员管理：独立于 Settings 弹窗的两个全局模态框。
    // memberManage 名册弹窗入口在 UserMenu（canManageMembers）；
    // memberAudit 行为日志弹窗挂在成员管理弹窗右上角链接上。
    // memberManage = 名册 + 加人/重置密码/邀请/统计/移出；
    // memberAudit = 成员行为日志（白话文案）。
    showMemberManageModal: false,
    showMemberAuditModal: false,
    manualEditorVisible: false,
    manualEditorMode: 'create' as 'create' | 'edit',
    manualEditorKBId: null as string | null,
    manualEditorKnowledgeId: null as string | null,
    manualEditorInitialTitle: '',
    manualEditorInitialContent: '',
    manualEditorInitialStatus: 'draft' as 'draft' | 'publish',
    manualEditorOnSuccess: null as null | ((payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => void),
    sidebarCollapsed: localStorage.getItem('sidebar_collapsed') === 'true'
  }),

  actions: {
    openSettings(section?: string, subSection?: string) {
      this.settingsInitialSection = section || null
      this.settingsInitialSubSection = subSection || null
      this.showSettingsModal = true
    },

    closeSettings() {
      this.showSettingsModal = false
      this.settingsInitialSection = null
      this.settingsInitialSubSection = null
    },

    // 成员管理弹窗（000101）：openMemberManage 时刷新名册由组件自己
    // watch visible 完成，store 只负责开关，不携带初始状态。
    openMemberManage() {
      this.showMemberManageModal = true
    },

    closeMemberManage() {
      this.showMemberManageModal = false
    },

    openMemberAudit() {
      this.showMemberAuditModal = true
    },

    closeMemberAudit() {
      this.showMemberAuditModal = false
    },

    toggleSettings() {
      this.showSettingsModal = !this.showSettingsModal
    },

    openKBSettings(kbId: string, initialSection?: string) {
      this.currentKBId = kbId
      this.kbEditorMode = 'edit'
       this.kbEditorType = 'document'
      this.kbEditorInitialSection = initialSection || null
      this.showKBEditorModal = true
    },

    openEditKB(kbId: string, initialSection?: string) {
      this.openKBSettings(kbId, initialSection)
    },

    openCreateKB(type: 'document' | 'faq' = 'document', initialSection?: string) {
      this.currentKBId = null
      this.kbEditorMode = 'create'
      this.kbEditorType = type
      this.kbEditorInitialSection = initialSection || null
      this.showKBEditorModal = true
    },

    closeKBEditor() {
      this.showKBEditorModal = false
      this.currentKBId = null
      this.kbEditorInitialSection = null
      this.kbEditorType = 'document'
    },

    openManualEditor(options: {
      mode?: 'create' | 'edit'
      kbId?: string | null
      knowledgeId?: string | null
      title?: string
      content?: string
      status?: 'draft' | 'publish'
      onSuccess?: (payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => void
    } = {}) {
      this.manualEditorMode = options.mode || 'create'
      this.manualEditorKBId = options.kbId ?? null
      this.manualEditorKnowledgeId = options.knowledgeId ?? null
      this.manualEditorInitialTitle = options.title || ''
      this.manualEditorInitialContent = options.content || ''
      this.manualEditorInitialStatus = options.status || 'draft'
      this.manualEditorOnSuccess = options.onSuccess || null
      this.manualEditorVisible = true
    },

    closeManualEditor() {
      this.manualEditorVisible = false
      this.manualEditorKnowledgeId = null
      this.manualEditorInitialContent = ''
      this.manualEditorInitialTitle = ''
      this.manualEditorInitialStatus = 'draft'
      this.manualEditorOnSuccess = null
    },

    notifyManualEditorSuccess(payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) {
      if (typeof this.manualEditorOnSuccess === 'function') {
        try {
          this.manualEditorOnSuccess(payload)
        } catch (err) {
          console.error('Manual editor success callback error:', err)
        }
      }
      this.manualEditorOnSuccess = null
    },

    // 设置当前选中的标签 ID
    toggleSelectedTagId(tagId: string) {
      const idx = this.selectedTagIds.indexOf(tagId)
      if (idx >= 0) {
        this.selectedTagIds.splice(idx, 1)
      } else {
        this.selectedTagIds.push(tagId)
      }
    },

    clearSelectedTagIds() {
      this.selectedTagIds = []
    },

    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      localStorage.setItem('sidebar_collapsed', String(this.sidebarCollapsed))
    },

    collapseSidebar() {
      this.sidebarCollapsed = true
      localStorage.setItem('sidebar_collapsed', 'true')
    },

    expandSidebar() {
      this.sidebarCollapsed = false
      localStorage.setItem('sidebar_collapsed', 'false')
    }
  }
})

