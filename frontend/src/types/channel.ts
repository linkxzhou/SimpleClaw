export type ChannelType =
  | 'telegram'
  | 'discord'
  | 'whatsapp'
  | 'feishu'

export type ChannelStatus = 'connected' | 'disconnected' | 'connecting' | 'error'

export type ChannelConnectionType = 'token' | 'qr' | 'oauth' | 'webhook'

export interface Channel {
  id: string
  type: ChannelType
  name: string
  status: ChannelStatus
  accountId?: string
  lastActivity?: string
  error?: string
  avatar?: string
  metadata?: Record<string, unknown>
}

export interface ChannelConfigField {
  key: string
  label: string
  type: 'text' | 'password' | 'select'
  placeholder?: string
  required?: boolean
  envVar?: string
  description?: string
  options?: { value: string; label: string }[]
}

export interface ChannelMeta {
  id: ChannelType
  name: string
  icon: string
  description: string
  connectionType: ChannelConnectionType
  docsUrl: string
  configFields: ChannelConfigField[]
  instructions: string[]
  isPlugin?: boolean
}

export const CHANNEL_ICONS: Record<ChannelType, string> = {
  telegram: '✈️',
  discord: '🎮',
  whatsapp: '📱',
  feishu: '🐦',
}

export const CHANNEL_NAMES: Record<ChannelType, string> = {
  telegram: 'Telegram',
  discord: 'Discord',
  whatsapp: 'WhatsApp',
  feishu: 'Feishu / Lark',
}

export const CHANNEL_META: Record<ChannelType, ChannelMeta> = {
  telegram: {
    id: 'telegram',
    name: 'Telegram',
    icon: '✈️',
    description: 'channels:meta.telegram.description',
    connectionType: 'token',
    docsUrl: 'channels:meta.telegram.docsUrl',
    configFields: [
      {
        key: 'botToken',
        label: 'channels:meta.telegram.fields.botToken.label',
        type: 'password',
        placeholder: 'channels:meta.telegram.fields.botToken.placeholder',
        required: true,
        envVar: 'TELEGRAM_BOT_TOKEN',
      },
      {
        key: 'allowedUsers',
        label: 'channels:meta.telegram.fields.allowedUsers.label',
        type: 'text',
        placeholder: 'channels:meta.telegram.fields.allowedUsers.placeholder',
        description: 'channels:meta.telegram.fields.allowedUsers.description',
        required: true,
      },
    ],
    instructions: [
      'channels:meta.telegram.instructions.0',
      'channels:meta.telegram.instructions.1',
      'channels:meta.telegram.instructions.2',
      'channels:meta.telegram.instructions.3',
      'channels:meta.telegram.instructions.4',
    ],
  },
  discord: {
    id: 'discord',
    name: 'Discord',
    icon: '🎮',
    description: 'channels:meta.discord.description',
    connectionType: 'token',
    docsUrl: 'channels:meta.discord.docsUrl',
    configFields: [
      {
        key: 'token',
        label: 'channels:meta.discord.fields.token.label',
        type: 'password',
        placeholder: 'channels:meta.discord.fields.token.placeholder',
        required: true,
        envVar: 'DISCORD_BOT_TOKEN',
      },
      {
        key: 'guildId',
        label: 'channels:meta.discord.fields.guildId.label',
        type: 'text',
        placeholder: 'channels:meta.discord.fields.guildId.placeholder',
        required: true,
        description: 'channels:meta.discord.fields.guildId.description',
      },
      {
        key: 'channelId',
        label: 'channels:meta.discord.fields.channelId.label',
        type: 'text',
        placeholder: 'channels:meta.discord.fields.channelId.placeholder',
        required: false,
        description: 'channels:meta.discord.fields.channelId.description',
      },
    ],
    instructions: [
      'channels:meta.discord.instructions.0',
      'channels:meta.discord.instructions.1',
      'channels:meta.discord.instructions.2',
      'channels:meta.discord.instructions.3',
      'channels:meta.discord.instructions.4',
      'channels:meta.discord.instructions.5',
    ],
  },
  whatsapp: {
    id: 'whatsapp',
    name: 'WhatsApp',
    icon: '📱',
    description: 'channels:meta.whatsapp.description',
    connectionType: 'qr',
    docsUrl: 'channels:meta.whatsapp.docsUrl',
    configFields: [],
    instructions: [
      'channels:meta.whatsapp.instructions.0',
      'channels:meta.whatsapp.instructions.1',
      'channels:meta.whatsapp.instructions.2',
      'channels:meta.whatsapp.instructions.3',
    ],
  },
  feishu: {
    id: 'feishu',
    name: 'Feishu / Lark',
    icon: '🐦',
    description: 'channels:meta.feishu.description',
    connectionType: 'token',
    docsUrl: 'channels:meta.feishu.docsUrl',
    configFields: [
      {
        key: 'appId',
        label: 'channels:meta.feishu.fields.appId.label',
        type: 'text',
        placeholder: 'channels:meta.feishu.fields.appId.placeholder',
        required: true,
        envVar: 'FEISHU_APP_ID',
      },
      {
        key: 'appSecret',
        label: 'channels:meta.feishu.fields.appSecret.label',
        type: 'password',
        placeholder: 'channels:meta.feishu.fields.appSecret.placeholder',
        required: true,
        envVar: 'FEISHU_APP_SECRET',
      },
    ],
    instructions: [
      'channels:meta.feishu.instructions.0',
      'channels:meta.feishu.instructions.1',
      'channels:meta.feishu.instructions.2',
      'channels:meta.feishu.instructions.3',
      'channels:meta.feishu.instructions.4',
    ],
    isPlugin: true,
  },
}

export function getPrimaryChannels(): ChannelType[] {
  return ['telegram', 'discord', 'whatsapp', 'feishu']
}

export function getAllChannels(): ChannelType[] {
  return Object.keys(CHANNEL_META) as ChannelType[]
}
