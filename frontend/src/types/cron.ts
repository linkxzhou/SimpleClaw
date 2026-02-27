import type { ChannelType } from './channel'

export interface CronJobTarget {
  channelType: ChannelType
  channelId: string
  channelName: string
}

export interface CronJobLastRun {
  time: string
  success: boolean
  error?: string
  duration?: number
}

export type CronSchedule =
  | { kind: 'at'; at: string }
  | { kind: 'every'; everyMs: number; anchorMs?: number }
  | { kind: 'cron'; expr: string; tz?: string }

export interface CronJob {
  id: string
  name: string
  message: string
  schedule: string | CronSchedule
  target: CronJobTarget
  enabled: boolean
  createdAt: string
  updatedAt: string
  lastRun?: CronJobLastRun
  nextRun?: string
}

export interface CronJobCreateInput {
  name: string
  message: string
  schedule: string
  target: CronJobTarget
  enabled?: boolean
}

export interface CronJobUpdateInput {
  name?: string
  message?: string
  schedule?: string
  target?: CronJobTarget
  enabled?: boolean
}

export type ScheduleType = 'daily' | 'weekly' | 'monthly' | 'interval' | 'custom'
