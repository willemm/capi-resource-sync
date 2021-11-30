
# TODO

* Check target resource for update (annotation with source resourceversion?)
* Set annotations like source cluster, etc.
* Set status on resourcelone object to see if clone was successful, etc
* On reconcile on resourceclone, add watch to the source object (remove on deletion)
* When resourceclone is being deleted, also delete target object (needs finalizer)
