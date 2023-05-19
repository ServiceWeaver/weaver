# Monolith or Microservices or Both

<div class="blog-author">Robert Grandl</div>
<div class="blog-date">May 16, 2023</div>

How to write distributed applications has been a long-standing and highly contentious topic. Proponents of microservices argue that breaking down an application into microservices brings benefits such as improved scalability, independent deployment, and easier maintenance. On the other hand, supporters of monolithic architecture argue that a single, cohesive system promotes simplicity, easier development, and reduced operational overhead. This heated debate reflects the diverse needs and perspectives of different teams as they strive to find the most suitable architectural approach for their specific projects.

We have written a vision [paper][vision_paper] that will appear at [HotOS'2023][hotos],
in which we describe our vision on how to write distributed applications. We argue
that developers should write their application as a modular binary and decide later
whether they really need to move to a microservices-based architecture. By postponing
the decision of how exactly to split into different microservices, it allows them
to write fewer and better microservices. We believe that this approach eases the
tension between the two different architectures, allowing developers to achieve
the best of both worlds.

<div class="note">
When writing a distributed application, conventional wisdom says to split your application into separate services
that can be rolled out independently. This approach is well intentioned, but a microservices-based architecture like this
often backfires, introducing challenges that counteract the
benefits the architecture tries to achieve. Fundamentally, this
is because microservices conflate logical boundaries (how
code is written) with physical boundaries (how code is deployed). In this paper, we propose a different programming
methodology that decouples the two in order to solve these
challenges. With our approach, developers write their applications as logical monoliths, offload the decisions of how to
distribute and run applications to an automated runtime, and
deploy applications atomically. Our prototype implementation reduces application latency by up to 15× and reduces
cost by up to 9× compared to the status quo.
</div>

Looking forward to an interesting discussion at [HotOS'23][hotos] about microservices versus monolithic architectures.

[hotos]: https://sigops.org/s/conferences/hotos/2023/
[vision_paper]: ../assets/docs/hotos23_vision_paper.pdf
