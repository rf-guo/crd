## 核心原理

informer监听特定resources的变化，驱动handler完成状态更新，监听过程包括edge-driven和level-driven，当resource本身发生更新时通知informer，执行update handler，这种称为edge-driven，然而，如果handler处理失败，这个event会发生丢失，kubernetes的方式是结合level-driven，执行 period reconcile，使得resource的状态和用户期望的状态最终保持一致。

也就是说kubernetes是有了edge-driven + level-driven俩种结合的方式保证resource的state和用户期望的状态做到最终一致。