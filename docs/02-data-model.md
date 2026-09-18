# 02 · 数据模型与事务

## 通用约定

主键采用应用生成 UUIDv7，时间 UTC `timestamptz`，整数 token，货币使用 `BIGINT` 微单位（1 unit = 10^-6 指定币种），价格使用 NUMERIC；对外金额/大整数以字符串表达避免 JS 精度丢失。所有币种显式，第一版一个租户只用一种结算币种，不做隐式汇率换算。

租户表之间使用 `(tenant_id,id)` 唯一约束与复合外键，不只依赖 handler 的 where。所有查询必须显式作用域；sqlc 管理查询与租户查询分开命名。RLS 可另加防御层，但不代替应用检查；第一版不把全权限 DB 账号称为“已经受 RLS 保护”。

## 表及最低字段

| 表 | 核心字段/约束 |
|---|---|
| tenants | id,name,status,currency,policy_version；禁用不删除账本 |
| users | tenant_id,id,display_name,status；无密码/社交登录 |
| projects / project_members | tenant_id,id；成员与权限作用域 |
| api_keys | tenant_id,project_id,user_id,public_id,secret_digest,digest_key_id,scopes,model_policy_id,status,expires_at,revoked_at,auth_version；public_id 全局唯一 |
| admin_principals / admin_tokens | 独立身份和 scopes；无默认管理员密码；不可用 public Key 认证 |
| authorization_records | provider,auth_mode,intended_use,allowed_tenants,source_refs,reviewed_by,reviewed_at,expires_at,status；仅表示运维核实记录，不生成外部授权 |
| providers | provider_type,allowed_origins,capability_version,authorization_record_id,status |
| upstream_accounts | provider_id,owner_scope,external_subject_fingerprint,quota_group_id,egress_profile_id,status,status_reason,version；同实际主体不能靠重复导入变成独立配额 |
| credentials | account_id,auth_mode,ciphertext,key_id,nonce,aad_version,credential_version,expires_at,status；秘密字段不在通用 JSONB |
| oauth_flows | tenant_id,admin_id,provider_id,state_digest,encrypted_verifier,redirect_uri,expires_at,status,claim_nonce；state 唯一 |
| oauth_refresh_attempts | credential_id,expected_version,attempt_nonce,owner_id,started_at,deadline,state,error_class；同凭据最多一个未决刷新 |
| egress_profiles | endpoint_policy,proxy_secret_ref,tls_policy,version,status；无明文代理密码 |
| quota_groups | provider / organization / account / model / reset domain；配额范围不可只按 key 区分 |
| model_catalog / model_routes | public_name,provider_model_id,protocol,capabilities,policy_version,priority,weight；已授权模型才出现在 /models |
| price_versions / price_items | model/tier/dimension,currency,unit_price,effective_at；发布后不可变 |
| requests | tenant/project/user/key,request_id,idempotency_digest,body_digest,model,protocol,status,dispatch_status,usage_status,started/finished/config snapshot；无正文 |
| request_attempts | request_id,attempt_no,account_id,credential_version,egress_version,upstream_request_id,dispatch_phase,result_class；(request_id,attempt_no) 唯一 |
| response_bindings | tenant_id,project_id,principal_scope,provider_id,provider_resource_id,account_id,credential_scope,expires_at；归属在转发 ID 前落盘 |
| usage_events | request/attempt,source,event_key,completeness,typed counts,allowlisted provider metadata；唯一事件键 |
| billing_accounts | tenant_id,currency,posted_balance_micros,held_micros,version；不能任意编辑余额 |
| balance_holds | request_id,account_id,amount_micros,state,created/deadline；一个请求一份当前有效预留 |
| journal_transactions / journal_entries | business_key 唯一；currency；每事务借贷平衡；不可 UPDATE/DELETE |
| settlements | request_id,price_snapshot,usage_event_id,amount_micros,status；基础结算唯一，更正另记调整分录 |
| admin_adjustments | actor,reason,external_reference,idempotency_key；引用账本，不是裸 UPDATE balance |
| audit_events | actor,action,target,reason,result,safe metadata；与敏感写事务原子落盘 |
| outbox_events | event_id,type,payload_ref,created_at,status；不携带密钥/正文 |
| rate_state_snapshots / scheduling_epochs | 必要的恢复标记、持久停用/冷却；不用于替代上游实际配额 |

River 使用它自己的受版本管理表；不能手写并冒充其内部 schema。只有一种事件发布路径：业务事务内插入 River job，或事务 outbox 再由 dispatcher 入队；选定一种并写 ADR，禁止同事件双发但无幂等保护。

## 事务边界

鉴权/授权撤销：更新版本+审计+失效事件原子提交。
余额预留：锁定单个 billing_account -> 校验 available -> 插入 hold -> held 增量；禁止在事务里等待上游或 Valkey。
结算：固定锁顺序（billing_account -> hold -> request -> settlement）-> 幂等检查 -> 记平衡分录 -> 更新余额投影 -> 释放 hold -> request 状态 -> 事件。
刷新：短事务 CAS claim；网络在事务外；带 expected_version / nonce 的短事务保存结果。

## 并发与迁移

SQL UNIQUE/CHECK/FK 保证核心不变量；序列化失败/死锁仅重试整个短数据库事务，不重发模型请求。所有金额加减检查溢出和非负输入。生成代码提交仓库；生成后 git diff 必须为空。

生产 migration 使用独立 migrator 账号和全局迁移锁，显式命令执行。应用启动只检查 schema 兼容性，不抢着迁移。采用 expand/contract，先加字段/兼容读写再清理；不可逆迁移必须单独标记，不提供虚假 down。

请求明细先普通索引和时间范围查询；达到压测证据后再分区，不在第一版假设无限查询。核心账本不能跟随日志 TTL 删除。
