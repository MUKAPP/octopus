import { useState } from "react"
import { motion, useReducedMotion } from "motion/react"
import { useTranslations } from 'use-intl'
import { Button } from "@/components/ui/button"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useLogin } from "@/api/user";
import { useAPIKeyLogin } from "@/api/apikey";
import Logo from "@/components/modules/logo"
import {
  ENTRANCE_VARIANTS,
  REDUCED_MOTION_ENTRANCE_VARIANTS,
} from '@/lib/animations/fluid-transitions'
import { AlertCircle, Eye, EyeOff, KeyRound, Loader2, User } from "lucide-react"
import {
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContents,
  TabsContent,
} from "@/components/animate-ui/components/animate/tabs"

type LoginMode = 'user' | 'apikey';

export function LoginForm({ onLoginSuccess }: { onLoginSuccess?: () => void }) {
  const t = useTranslations('login')
  const tCommon = useTranslations('common')
  const shouldReduceMotion = useReducedMotion() ?? false
  const [mode, setMode] = useState<LoginMode>('user')
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [trustDevice, setTrustDevice] = useState(false)
  const [apiKey, setApiKey] = useState("")
  const [showApiKey, setShowApiKey] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loginMutation = useLogin()
  const apiKeyLoginMutation = useAPIKeyLogin()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    try {
      if (mode === 'user') {
        await loginMutation.mutateAsync({
          username,
          password,
          expire: trustDevice ? -1 : 86400,
        })
      } else {
        await apiKeyLoginMutation.mutateAsync(apiKey)
      }

      onLoginSuccess?.()
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : t('error.generic')
      setError(errorMessage)
    }
  }

  const isPending = loginMutation.isPending || apiKeyLoginMutation.isPending

  const handleModeChange = (value: string) => {
    setMode(value as LoginMode)
    setError(null)
  }

  return (
    <div className="min-h-dvh flex items-center justify-center pl-[calc(1rem+var(--safe-area-left))] pr-[calc(1rem+var(--safe-area-right))] pt-[max(1.5rem,var(--safe-area-top))] pb-[max(1.5rem,var(--safe-area-bottom))] text-foreground">
      <motion.div
        variants={shouldReduceMotion ? REDUCED_MOTION_ENTRANCE_VARIANTS.content : ENTRANCE_VARIANTS.content}
        initial="initial"
        animate="animate"
        className="w-full max-w-[392px] rounded-3xl border border-border bg-card p-6 text-card-foreground shadow-sm sm:p-7"
      >
        <header className="mb-6 flex flex-col items-center gap-2.5 text-center">
          <Logo size={44} />
          <h1 className="text-2xl font-bold tracking-tight">Octopus</h1>
        </header>

        <Tabs value={mode} onValueChange={handleModeChange} className="gap-5">
          <TabsList className="w-full dark:bg-background/50">
            <TabsTrigger value="user" className="gap-2">
              <User className="size-4" />
              {t('mode.user')}
            </TabsTrigger>
            <TabsTrigger value="apikey" className="gap-2">
              <KeyRound className="size-4" />
              {t('mode.apikey')}
            </TabsTrigger>
          </TabsList>

          <form onSubmit={handleSubmit} className="space-y-4">
            <TabsContents className="-mx-1.5 px-1.5 py-1">
              <TabsContent value="user" className="space-y-4" style={{ overflow: 'visible' }}>
                <Field className="gap-1.5">
                  <FieldLabel htmlFor="username" className="text-xs font-medium text-muted-foreground">
                    {t('username')}
                  </FieldLabel>
                  <Input
                    id="username"
                    type="text"
                    autoComplete="username"
                    placeholder={t('usernamePlaceholder')}
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    required={mode === 'user'}
                    disabled={isPending}
                    className="h-9 rounded-xl text-base sm:text-sm"
                  />
                </Field>
                <Field className="gap-1.5">
                  <FieldLabel htmlFor="password" className="text-xs font-medium text-muted-foreground">
                    {t('password')}
                  </FieldLabel>
                  <div className="relative">
                    <Input
                      id="password"
                      type={showPassword ? "text" : "password"}
                      autoComplete="current-password"
                      placeholder={t('passwordPlaceholder')}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      required={mode === 'user'}
                      disabled={isPending}
                      className="h-9 rounded-xl pr-10 text-base sm:text-sm"
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword((prev) => !prev)}
                      disabled={isPending}
                      aria-label={showPassword ? tCommon('hidePassword') : tCommon('showPassword')}
                      aria-pressed={showPassword}
                      className="absolute right-1.5 top-1/2 -translate-y-1/2 inline-flex size-7 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring motion-reduce:transition-none"
                    >
                      {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </button>
                  </div>
                </Field>
                <div className="flex items-center justify-between gap-3 pt-0.5">
                  <FieldLabel
                    htmlFor="trust-device"
                    className="flex-1 cursor-pointer text-sm font-normal text-foreground/85"
                  >
                    {t('trustDevice')}
                  </FieldLabel>
                  <Switch
                    id="trust-device"
                    checked={trustDevice}
                    onCheckedChange={setTrustDevice}
                    disabled={isPending}
                  />
                </div>
              </TabsContent>
              <TabsContent value="apikey" style={{ overflow: 'visible' }}>
                <Field className="gap-1.5">
                  <FieldLabel htmlFor="apikey" className="text-xs font-medium text-muted-foreground">
                    {t('apikey')}
                  </FieldLabel>
                  <div className="relative">
                    <Input
                      id="apikey"
                      type={showApiKey ? "text" : "password"}
                      autoComplete="off"
                      placeholder={t('apikeyPlaceholder')}
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      required={mode === 'apikey'}
                      disabled={isPending}
                      className="h-9 rounded-xl pr-10 text-base sm:text-sm"
                    />
                    <button
                      type="button"
                      onClick={() => setShowApiKey((prev) => !prev)}
                      disabled={isPending}
                      aria-label={showApiKey ? tCommon('hidePassword') : tCommon('showPassword')}
                      aria-pressed={showApiKey}
                      className="absolute right-1.5 top-1/2 -translate-y-1/2 inline-flex size-7 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring motion-reduce:transition-none"
                    >
                      {showApiKey ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </button>
                  </div>
                </Field>
              </TabsContent>
            </TabsContents>

            {error && (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-xl border border-destructive/30 bg-destructive/10 px-3 py-2.5 text-xs text-destructive"
              >
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <span className="leading-relaxed">{error}</span>
              </div>
            )}

            <Button
              type="submit"
              disabled={isPending}
              className="h-9 w-full rounded-xl shadow-xs active:scale-[0.99] motion-reduce:transform-none"
            >
              {isPending && <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />}
              {isPending ? t('button.loading') : t('button.submit')}
            </Button>
          </form>
        </Tabs>
      </motion.div>
    </div>
  )
}
