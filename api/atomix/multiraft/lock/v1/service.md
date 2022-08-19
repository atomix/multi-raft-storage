# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [atomix/multiraft/lock/v1/service.proto](#atomix_multiraft_lock_v1_service-proto)
    - [AcquireRequest](#atomix-multiraft-lock-v1-AcquireRequest)
    - [AcquireResponse](#atomix-multiraft-lock-v1-AcquireResponse)
    - [GetLockRequest](#atomix-multiraft-lock-v1-GetLockRequest)
    - [GetLockResponse](#atomix-multiraft-lock-v1-GetLockResponse)
    - [ReleaseRequest](#atomix-multiraft-lock-v1-ReleaseRequest)
    - [ReleaseResponse](#atomix-multiraft-lock-v1-ReleaseResponse)
  
    - [Lock](#atomix-multiraft-lock-v1-Lock)
  
- [Scalar Value Types](#scalar-value-types)



<a name="atomix_multiraft_lock_v1_service-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## atomix/multiraft/lock/v1/service.proto



<a name="atomix-multiraft-lock-v1-AcquireRequest"></a>

### AcquireRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [atomix.multiraft.v1.CommandRequestHeaders](#atomix-multiraft-v1-CommandRequestHeaders) |  |  |
| input | [AcquireInput](#atomix-multiraft-lock-v1-AcquireInput) |  |  |






<a name="atomix-multiraft-lock-v1-AcquireResponse"></a>

### AcquireResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [atomix.multiraft.v1.CommandResponseHeaders](#atomix-multiraft-v1-CommandResponseHeaders) |  |  |
| output | [AcquireOutput](#atomix-multiraft-lock-v1-AcquireOutput) |  |  |






<a name="atomix-multiraft-lock-v1-GetLockRequest"></a>

### GetLockRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [atomix.multiraft.v1.QueryRequestHeaders](#atomix-multiraft-v1-QueryRequestHeaders) |  |  |
| input | [GetLockInput](#atomix-multiraft-lock-v1-GetLockInput) |  |  |






<a name="atomix-multiraft-lock-v1-GetLockResponse"></a>

### GetLockResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [atomix.multiraft.v1.QueryResponseHeaders](#atomix-multiraft-v1-QueryResponseHeaders) |  |  |
| output | [GetLockOutput](#atomix-multiraft-lock-v1-GetLockOutput) |  |  |






<a name="atomix-multiraft-lock-v1-ReleaseRequest"></a>

### ReleaseRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [atomix.multiraft.v1.CommandRequestHeaders](#atomix-multiraft-v1-CommandRequestHeaders) |  |  |
| input | [ReleaseInput](#atomix-multiraft-lock-v1-ReleaseInput) |  |  |






<a name="atomix-multiraft-lock-v1-ReleaseResponse"></a>

### ReleaseResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [atomix.multiraft.v1.CommandResponseHeaders](#atomix-multiraft-v1-CommandResponseHeaders) |  |  |
| output | [ReleaseOutput](#atomix-multiraft-lock-v1-ReleaseOutput) |  |  |





 

 

 


<a name="atomix-multiraft-lock-v1-Lock"></a>

### Lock
Lock is a service for a counter primitive

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Acquire | [AcquireRequest](#atomix-multiraft-lock-v1-AcquireRequest) | [AcquireResponse](#atomix-multiraft-lock-v1-AcquireResponse) | Acquire attempts to acquire the lock |
| Release | [ReleaseRequest](#atomix-multiraft-lock-v1-ReleaseRequest) | [ReleaseResponse](#atomix-multiraft-lock-v1-ReleaseResponse) | Release releases the lock |
| GetLock | [GetLockRequest](#atomix-multiraft-lock-v1-GetLockRequest) | [GetLockResponse](#atomix-multiraft-lock-v1-GetLockResponse) | GetLock gets the lock state |

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |
