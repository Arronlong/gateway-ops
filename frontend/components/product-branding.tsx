import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react"
import { useLocation } from "react-router-dom"
import { apiFetch } from "@/lib/api"
import { brandingPageTitle, defaultProductIcon, normalizeBranding, type ProductBranding } from "@/lib/branding"
import { cn } from "@/lib/utils"

const defaultBranding=normalizeBranding()
const BrandContext=createContext({branding:defaultBranding,updateBranding:(_value:ProductBranding)=>{},setPageTitle:(_value:{path:string;title:string}|null)=>{}})
const titles:Record<string,string>={"/":"运营总览","/channels":"渠道管理","/relay-stations":"中转站管理","/pelican":"鹈鹕测智","/settings":"系统设置","/captcha":"验证码服务","/alerts":"预警中心","/notifications":"预警中心","/operations/costs":"成本管理","/operations/local-pool":"本地号池","/public/service-status":"AI 服务状态"}

export function ProductBrandingProvider({children}:{children:ReactNode}) {
  const {pathname}=useLocation()
  const [branding,setBranding]=useState(defaultBranding)
  const [customTitle,setPageTitle]=useState<{path:string;title:string}|null>(null)
  const updateBranding=useCallback((value:ProductBranding)=>setBranding(normalizeBranding(value)),[])
  useEffect(()=>{
    const controller=new AbortController()
    apiFetch<ProductBranding>("/public/branding",{signal:controller.signal,skipAuthErrorHandler:true}).then(updateBranding).catch(()=>{})
    return()=>controller.abort()
  },[updateBranding])
  const title=customTitle?.path===pathname?customTitle.title:titles[pathname]??(pathname.startsWith("/public/model-pricing")?"模型价格":pathname.startsWith("/public/relay-monitor")?"中转站监控":"")
  useEffect(()=>{document.title=brandingPageTitle(title,branding.product_name)},[title,branding.product_name])
  useEffect(()=>{
    const icon=branding.product_icon||defaultProductIcon
    const applyIcon=(source:string)=>document.querySelectorAll<HTMLLinkElement>('link[rel="icon"],link[rel="apple-touch-icon"]').forEach(link=>{
      link.href=source;link.removeAttribute("media");link.type=source===defaultProductIcon?"image/svg+xml":"image/png"
    })
    applyIcon(icon)
    if(!branding.product_icon)return
    const image=new Image()
    image.onerror=()=>applyIcon(defaultProductIcon)
    image.src=icon
    return()=>{image.onerror=null}
  },[branding.product_icon])
  const value=useMemo(()=>({branding,updateBranding,setPageTitle}),[branding,updateBranding])
  return <BrandContext.Provider value={value}>{children}</BrandContext.Provider>
}

export function useProductBranding(){return useContext(BrandContext)}

export function useProductPageTitle(title:string){
  const {pathname}=useLocation()
  const {setPageTitle}=useProductBranding()
  useEffect(()=>{setPageTitle({path:pathname,title});return()=>setPageTitle(null)},[pathname,title,setPageTitle])
}

export function ProductIcon({className="size-5",icon}:{className?:string;icon?:string}){
  const {branding}=useProductBranding()
  const source=icon??branding.product_icon
  const [failedSource,setFailedSource]=useState<string|null>(null)
  return <img src={source&&source!==failedSource?source:defaultProductIcon} alt="" aria-hidden="true" data-product-icon className={cn("inline-block shrink-0 rounded-md object-contain",className)} onError={()=>setFailedSource(source)}/>
}
