export interface Skill {
  id: string
  slug?: string
  name: string
  description: string
  enabled: boolean
  icon?: string
  version?: string
  author?: string
  configurable?: boolean
  config?: Record<string, unknown>
  isCore?: boolean
  isBundled?: boolean
  dependencies?: string[]
}

export interface SkillBundle {
  id: string
  name: string
  nameZh: string
  description: string
  descriptionZh: string
  icon: string
  skills: string[]
  recommended?: boolean
}

export interface MarketplaceSkill {
  slug: string
  name: string
  description: string
  version: string
  author?: string
  downloads?: number
  stars?: number
}

export interface SkillConfigSchema {
  type: 'object'
  properties: Record<string, {
    type: 'string' | 'number' | 'boolean' | 'array'
    title?: string
    description?: string
    default?: unknown
    enum?: unknown[]
  }>
  required?: string[]
}
