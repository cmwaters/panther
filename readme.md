# Nova

Nova is meant to be a foundation layer for building consensus engines. We break down what is required to build a consensus engine into a few primitives, first there is p2p, all consensus engines require reliable communication between peers and modules may require access to p2p network but modules should not rely on each other to execute their work.

Consensus is the operation of coming to agreement on data. The data is not a concern of consensus. Modules & Consensus should not rely on each other and the application should be the glue between services.

Goals for this work:

* Develop a pluggable consensus & networking engine that comes to consensus on bytes
* Leave block and head construction to the application
* Operate with minimal resource consumption
