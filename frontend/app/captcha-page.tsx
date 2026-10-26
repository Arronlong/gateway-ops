import { ProductIcon } from "@/components/product-branding"
import { ShieldCheck } from "lucide-react"
import { CaptchaStatus } from "@/components/monitor/bottom-panels"

export default function CaptchaPage() {
  return (
    <section className="space-y-4">
      <header className="list-panel-controls flex flex-wrap items-end justify-between gap-3 border-l-2 border-brand pl-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-bold text-foreground"><ProductIcon className="size-5 sm:size-6"/>验证码服务</h1>
          <p className="mt-1 text-xs text-muted-foreground">
            {"配置 CapSolver、2Captcha 等服务，为启用 Turnstile 的渠道自动获取验证令牌。"}
          </p>
        </div>
      </header>
      <CaptchaStatus fullPage />
    </section>
  )
}
