import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import {pelicanRunOptions,hasConfiguredPelicanPlan,normalizePelicanPlans,defaultPelicanPlan} from "./pelican.ts"

test("禁用方案不出现在手动执行列表并保留开关配置",()=>{
 const settings={pelican_plans:[{model:"disabled-model",prompt:"draw",reasoning_effort:"high",enabled:false},{model:"enabled-model",prompt:"draw",reasoning_effort:"low",enabled:true},{model:"legacy-model",prompt:"draw",reasoning_effort:"medium"}]}
 assert.deepEqual(pelicanRunOptions(settings).map(plan=>plan.model),["enabled-model","legacy-model"])
 assert.equal(normalizePelicanPlans(settings)[0].enabled,false)
 assert.equal(normalizePelicanPlans(settings)[1].enabled,true)
 assert.equal(defaultPelicanPlan().enabled,true)
 assert.equal(hasConfiguredPelicanPlan({pelican_plans:settings.pelican_plans.slice(0,1)}),false)
 assert.equal(hasConfiguredPelicanPlan(settings),true)
})
