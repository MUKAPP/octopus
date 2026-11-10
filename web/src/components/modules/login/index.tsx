import { useState } from "react"
import { motion, useReducedMotion } from "motion/react"
import { useTranslations } from 'use-intl'
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useLogin } from "@/api/user";
import { useAPIKeyLogin } from "@/api/apikey";
import Logo from "@/components/modules/logo"
import { DURATION, REDUCED_MOTION_TRANSITION } from '@/lib/animations/fluid-transitions'
import { Eye, EyeOff, KeyRound, User } from "lucide-react"
import {
  Tabs,
  TabsList,
  TabsHighlight,
  TabsHighlightItem,
  TabsTrigger,
  TabsContents,
  TabsContent,
} from "@/components/animate-ui/primitives/animate/tabs"

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
    <motion.div
      initial={shouldReduceMotion ? { opacity: 1 } : { opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: DURATION.page }}
      className="min-h-dvh flex items-center justify-center pl-[calc(1.5rem+var(--safe-area-left))] pr-[calc(1.5rem+var(--safe-area-right))] pt-[max(1.5rem,var(--safe-area-top))] pb-[max(1.5rem,var(--safe-area-bottom))] text-foreground"
    >
      <div className="w-full max-w-sm space-y-8">
        <header className="flex flex-col items-center gap-3">
          <Logo size={48} />
          <h1 className="text-2xl font-bold">Octopus</h1>
        </header>

        <Tabs value={mode} onValueChange={handleModeChange}>
          <TabsList className="flex p-1 bg-muted rounded-2xl">
            <TabsHighlight className="rounded-xl bg-background shadow-sm">
              <TabsHighlightItem value="user" className="flex-1">
                <TabsTrigger
                  value="user"
                  className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-xl text-sm font-medium transition-colors data-[state=active]:text-foreground data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:text-foreground"
                >
                  <User className="w-4 h-4" />
                  {t('mode.user')}
                </TabsTrigger>
              </TabsHighlightItem>
              <TabsHighlightItem value="apikey" className="flex-1">
                <TabsTrigger
                  value="apikey"
                  className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-xl text-sm font-medium transition-colors data-[state=active]:text-foreground data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:text-foreground"
                >
                  <KeyRound className="w-4 h-4" />
                  {t('mode.apikey')}
                </TabsTrigger>
              </TabsHighlightItem>
            </TabsHighlight>
          </TabsList>

          <form onSubmit={handleSubmit} className="space-y-6 pt-2">
            <TabsContents className="p-3 -mx-3 py-6">
              <TabsContent value="user" className="space-y-6" style={{ overflow: 'visible' }}>
                <Field>
                  <FieldLabel htmlFor="username">{t('username')}</FieldLabel>
                  <Input
                    id="username"
                    type="text"
                    autoComplete="username"
                    placeholder={t('usernamePlaceholder')}
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    required={mode === 'user'}
                    disabled={isPending}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="password">{t('password')}</FieldLabel>
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
                      className="pr-10"
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword((prev) => !prev)}
                      disabled={isPending}
                      aria-label={showPassword ? tCommon('hidePassword') : tCommon('showPassword')}
                      aria-pressed={showPassword}
                      className="absolute right-1.5 top-1/2 -translate-y-1/2 inline-flex size-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring motion-reduce:transition-none"
                    >
                      {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </button>
                  </div>
                </Field>
                <Field orientation="horizontal" data-disabled={isPending}>
                  <input
                    id="trust-device"
                    type="checkbox"
                    checked={trustDevice}
                    onChange={(e) => setTrustDevice(e.target.checked)}
                    disabled={isPending}
                    className="h-4 w-4 rounded border-input accent-primary"
                  />
                  <FieldLabel htmlFor="trust-device" className="text-muted-foreground">
                    {t('trustDevice')}
                  </FieldLabel>
                </Field>
              </TabsContent>
              <TabsContent value="apikey" style={{ overflow: 'visible' }}>
                <Field>
                  <FieldLabel htmlFor="apikey">{t('apikey')}</FieldLabel>
                  <Input
                    id="apikey"
                    type="password"
                    autoComplete="off"
                    placeholder={t('apikeyPlaceholder')}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    required={mode === 'apikey'}
                    disabled={isPending}
                  />
                </Field>
              </TabsContent>
            </TabsContents>

            {error && <FieldDescription className="text-destructive">{error}</FieldDescription>}

            <Button type="submit" disabled={isPending} className="w-full">
              {isPending ? t('button.loading') : t('button.submit')}
            </Button>
          </form>
        </Tabs>
      </div>
    </motion.div>
  )
}
